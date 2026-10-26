package daemon

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"val2harness/internal/config"
)

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

func IsProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func GetStatus(addr string) (bool, int) {
	pidPath, err := getPIDPath()
	if err != nil {
		return false, 0
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		return false, 0
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || !IsProcessAlive(pid) {
		_ = os.Remove(pidPath)
		return false, 0
	}

	// Verify HTTP responds
	client := &http.Client{Timeout: 1 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err == nil {
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK, pid
	}

	return false, pid
}

func StartDaemon(addr string) (int, error) {
	running, pid := GetStatus(addr)
	if running {
		return pid, fmt.Errorf("server already running with PID %d", pid)
	}

	binPath, err := os.Executable()
	if err != nil {
		return 0, err
	}

	logPath, err := getLogPath()
	if err != nil {
		return 0, err
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return 0, err
	}

	cmd := exec.Command(binPath, "serve", "--addr", addr)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to spawn daemon: %w", err)
	}

	pid = cmd.Process.Pid
	pidPath, _ := getPIDPath()
	_ = os.WriteFile(pidPath, []byte(strconv.Itoa(pid)), 0644)

	// Wait up to 3s for healthz
	client := &http.Client{Timeout: 500 * time.Millisecond}
	healthy := false
	for range 6 {
		time.Sleep(500 * time.Millisecond)
		resp, err := client.Get("http://" + addr + "/healthz")
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			healthy = true
			break
		}
	}

	if !healthy {
		return pid, fmt.Errorf("process started (PID %d) but health check failed; check %s", pid, logPath)
	}

	return pid, nil
}

func StopDaemon(addr string) error {
	pidPath, err := getPIDPath()
	if err != nil {
		return err
	}

	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("no running daemon found")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		_ = os.Remove(pidPath)
		return fmt.Errorf("invalid PID in pidfile")
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		_ = os.Remove(pidPath)
		return nil
	}

	_ = proc.Signal(syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	if IsProcessAlive(pid) {
		_ = proc.Signal(syscall.SIGKILL)
	}

	_ = os.Remove(pidPath)
	return nil
}
