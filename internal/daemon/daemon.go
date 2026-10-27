package daemon

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"val2harness/internal/config"
	"val2harness/internal/configfile"
)

// The identity binds a PID to its creation time and executable, rather than to
// whichever process happens to own the same numeric PID later.
type pidRecord struct {
	PID          int    `json:"pid"`
	Identity     string `json:"identity"`
	Addr         string `json:"addr"`
	ControlToken string `json:"control_token,omitempty"`
}

const ControlEnvKey = "VAL2HARNESS_DAEMON_CONTROL"

func CurrentProcessIdentity() (string, error) { return processIdentity(os.Getpid()) }

func reusedPID(record pidRecord) bool {
	if record.Identity == "" {
		return false
	}
	actual, err := processIdentity(record.PID)
	return err == nil && actual != record.Identity
}

func getPIDPath() (string, error) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.pid"), nil
}
func getLogPath() (string, error) {
	dir, err := config.GetConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.log"), nil
}
func readPID(path string) (pidRecord, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return pidRecord{}, nil, err
	}
	var record pidRecord
	if err = json.Unmarshal(data, &record); err != nil {
		// Older versions stored only an integer. Retain liveness information, but
		// never treat that integer alone as permission to terminate a process.
		record.PID, err = strconv.Atoi(strings.TrimSpace(string(data)))
	}
	if err != nil || record.PID <= 0 {
		return pidRecord{}, data, fmt.Errorf("invalid PID in %s", path)
	}
	return record, data, nil
}
func removeRecord(path string, data []byte) error {
	current, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !bytes.Equal(current, data) {
		return fmt.Errorf("daemon PID record changed; retry the command")
	}
	return os.Remove(path)
}

// Status can run concurrently with a start. Recheck under the lifecycle lock
// before removing anything so stale status cannot discard a newly written PID.
func discardStaleRecord(path string, data []byte) {
	unlock, err := lifecycleLock(path)
	if err != nil {
		return
	}
	defer unlock()
	record, current, err := readPID(path)
	if bytes.Equal(current, data) && (err != nil || !IsProcessAlive(record.PID) || reusedPID(record)) {
		_ = removeRecord(path, data)
	}
}

func GetStatus(addr string) (bool, int) {
	path, err := getPIDPath()
	if err != nil {
		return false, 0
	}
	record, data, err := readPID(path)
	if err != nil {
		if data != nil {
			discardStaleRecord(path, data)
		}
		return false, 0
	}
	if !IsProcessAlive(record.PID) {
		discardStaleRecord(path, data)
		return false, 0
	}
	if record.Identity != "" {
		identity, err := processIdentity(record.PID)
		if err != nil || identity != record.Identity {
			if err == nil {
				discardStaleRecord(path, data)
				return false, 0
			}
			return false, record.PID
		}
	}
	if record.Addr != "" && config.LocalBaseURL(record.Addr) != config.LocalBaseURL(addr) {
		return false, record.PID
	}
	if record.ControlToken != "" {
		return controlRequest(record, http.MethodGet) == nil, record.PID
	}
	return healthy(addr), record.PID
}

func controlRequest(record pidRecord, method string) error {
	if record.ControlToken == "" || record.Identity == "" || record.Addr == "" {
		return fmt.Errorf("no daemon control capability is available")
	}
	client := &http.Client{Timeout: 500 * time.Millisecond, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	req, err := http.NewRequest(method, config.LocalBaseURL(record.Addr)+"/api/local/daemon-control", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+record.ControlToken)
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("daemon control returned HTTP %d", response.StatusCode)
	}
	var identity struct {
		PID      int    `json:"pid"`
		Identity string `json:"identity"`
		Ready    bool   `json:"ready"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&identity); err != nil {
		return fmt.Errorf("invalid daemon control response: %w", err)
	}
	if identity.PID != record.PID || identity.Identity != record.Identity || !identity.Ready {
		return fmt.Errorf("daemon control response does not identify the recorded child")
	}
	return nil
}

func waitForRecordedExit(record pidRecord, timeout time.Duration) bool {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for {
		if !IsProcessAlive(record.PID) || reusedPID(record) {
			return true
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			return false
		}
	}
}
func healthy(addr string) bool {
	client := &http.Client{Timeout: 300 * time.Millisecond, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Get(config.LocalBaseURL(addr) + "/healthz")
	if err != nil {
		return false
	}
	defer response.Body.Close()
	return response.StatusCode == http.StatusOK
}
func lifecycleLock(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("create daemon directory: %w", err)
	}
	file, err := os.OpenFile(path+".lifecycle.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open daemon lifecycle lock: %w", err)
	}
	if err := lockFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	return func() { unlockFile(file); _ = file.Close() }, nil
}
func StartDaemon(addr string) (int, error) {
	path, err := getPIDPath()
	if err != nil {
		return 0, err
	}
	unlock, err := lifecycleLock(path)
	if err != nil {
		return 0, err
	}
	defer unlock()
	record, data, readErr := readPID(path)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) && data == nil {
		return 0, fmt.Errorf("read daemon PID: %w", readErr)
	}
	if readErr == nil && IsProcessAlive(record.PID) && !reusedPID(record) {
		return record.PID, fmt.Errorf("daemon PID %d is still alive; check its address, health, and local key before restarting", record.PID)
	}
	if data != nil {
		if err := removeRecord(path, data); err != nil {
			return 0, fmt.Errorf("remove stale daemon PID: %w", err)
		}
	}
	// Do not mistake an unrelated listener's health response for this child's.
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return 0, fmt.Errorf("cannot listen on %s: %w", addr, err)
	}
	_ = listener.Close()
	binary, err := os.Executable()
	if err != nil {
		return 0, err
	}
	logPath, err := getLogPath()
	if err != nil {
		return 0, err
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return 0, fmt.Errorf("open daemon log: %w", err)
	}
	defer logFile.Close()
	cmd := exec.Command(binary, "serve", "--addr", addr)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return 0, fmt.Errorf("create daemon capability: %w", err)
	}
	controlToken := base64.RawURLEncoding.EncodeToString(secret)
	for _, entry := range os.Environ() {
		if !strings.EqualFold(strings.SplitN(entry, "=", 2)[0], ControlEnvKey) {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, ControlEnvKey+"="+controlToken)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	setDaemonSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("spawn daemon: %w", err)
	}
	// Wait reaps the child and closes its process handle, including if the CLI
	// remains open after StartDaemon returns. Child stdout/stderr own inherited
	// handles, so the parent's copy of the log can be closed immediately.
	_ = logFile.Close()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	pid := cmd.Process.Pid
	identity, err := processIdentity(pid)
	if err != nil {
		_ = cmd.Process.Kill()
		<-exited
		return pid, fmt.Errorf("identify daemon (it may have exited); check %s: %w", logPath, err)
	}
	record = pidRecord{PID: pid, Identity: identity, Addr: addr, ControlToken: controlToken}
	data, err = json.Marshal(record)
	if err == nil {
		err = configfile.Write(path, data)
	}
	if err != nil {
		_ = cmd.Process.Kill()
		<-exited
		return pid, fmt.Errorf("persist daemon PID; child stopped: %w", err)
	}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case exitErr := <-exited:
			_ = removeRecord(path, data)
			return pid, fmt.Errorf("daemon exited before becoming healthy (%v); check %s", exitErr, logPath)
		default:
		}
		if controlRequest(record, http.MethodGet) == nil && IsProcessAlive(pid) {
			return pid, nil
		}
		select {
		case exitErr := <-exited:
			_ = removeRecord(path, data)
			return pid, fmt.Errorf("daemon exited before becoming healthy (%v); check %s", exitErr, logPath)
		case <-deadline.C:
			return pid, fmt.Errorf("daemon PID %d is alive but health check failed at %s; check %s and stop it before restarting", pid, addr, logPath)
		case <-ticker.C:
		}
	}
}
func StopDaemon(addr string) error {
	path, err := getPIDPath()
	if err != nil {
		return err
	}
	unlock, err := lifecycleLock(path)
	if err != nil {
		return err
	}
	defer unlock()
	record, data, err := readPID(path)
	if err != nil {
		if data != nil {
			_ = removeRecord(path, data)
		}
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("no running daemon found")
		}
		return err
	}
	if !IsProcessAlive(record.PID) {
		return removeRecord(path, data)
	}
	if reusedPID(record) {
		if err := removeRecord(path, data); err != nil {
			return err
		}
		return fmt.Errorf("removed stale daemon PID record; the unrelated process was left running")
	}
	if record.Identity == "" {
		return fmt.Errorf("cannot verify legacy daemon PID %d; stop the original process manually before restarting", record.PID)
	}
	if record.PID == os.Getpid() {
		return fmt.Errorf("refusing to stop the current management process")
	}
	requestErr := controlRequest(record, http.MethodGet)
	if requestErr == nil {
		requestErr = controlRequest(record, http.MethodPost)
		if waitForRecordedExit(record, 6*time.Second) {
			return removeRecord(path, data)
		}
	}
	if err := terminateProcess(record.PID, record.Identity); err != nil {
		return fmt.Errorf("could not stop daemon PID %d safely (control: %v): %w", record.PID, requestErr, err)
	}
	return removeRecord(path, data)
}
