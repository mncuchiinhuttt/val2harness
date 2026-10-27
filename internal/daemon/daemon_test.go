package daemon

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test executable doubles as a fake local gateway. It never loads account
// credentials, reads real configuration, or contacts an upstream service.
func TestMain(m *testing.M) {
	if os.Getenv("VAL2HARNESS_DAEMON_TEST_HELPER") == "1" && len(os.Args) > 1 && os.Args[1] == "serve" {
		if os.Getenv("VAL2HARNESS_DAEMON_TEST_EXIT") == "1" {
			os.Exit(9)
		}
		addr := os.Args[3]
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"status":"ok"}`) })
		token := os.Getenv(ControlEnvKey)
		identity, _ := processIdentity(os.Getpid())
		stop := make(chan struct{}, 1)
		mux.HandleFunc("/api/local/daemon-control", func(w http.ResponseWriter, r *http.Request) {
			if token == "" || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
				w.WriteHeader(401)
				return
			}
			if r.Method == http.MethodGet && os.Getenv("VAL2HARNESS_DAEMON_TEST_NO_CONTROL") == "1" {
				w.WriteHeader(404)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"pid": os.Getpid(), "identity": identity, "ready": true})
			if r.Method == http.MethodPost {
				select {
				case stop <- struct{}{}:
				default:
				}
			}
		})
		server := &http.Server{Addr: addr, Handler: mux}
		exited := make(chan error, 1)
		go func() { exited <- server.ListenAndServe() }()
		select {
		case err := <-exited:
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		case <-stop:
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		_ = server.Shutdown(ctx)
		cancel()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func isolatedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("VAL2HARNESS_DAEMON_TEST_HELPER", "1")
	return filepath.Join(home, ".config", "val2harness")
}
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	l.Close()
	return addr
}
func legacyPID(t *testing.T, pid int) string {
	t.Helper()
	path, err := getPIDPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(pid)), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}
func helperProcess(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "serve", "--addr", freeAddr(t))
	setDaemonSysProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}
func TestCurrentProcessLiveness(t *testing.T) {
	if !IsProcessAlive(os.Getpid()) {
		t.Fatal("current running process was reported dead")
	}
	for _, pid := range []int{0, -1, -200} {
		if IsProcessAlive(pid) {
			t.Errorf("invalid PID %d reported alive", pid)
		}
	}
}
func TestStatusRetainsAliveUnhealthyPID(t *testing.T) {
	isolatedHome(t)
	path := legacyPID(t, os.Getpid())
	healthy, pid := GetStatus(freeAddr(t))
	if healthy || pid != os.Getpid() {
		t.Fatalf("want unhealthy current PID, got %v %d", healthy, pid)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("live PID record lost: %v", err)
	}
}
func TestStartRefusesAliveUnhealthyPID(t *testing.T) {
	isolatedHome(t)
	child := helperProcess(t)
	legacyPID(t, child.Process.Pid)
	pid, err := StartDaemon(freeAddr(t))
	if err == nil || pid != child.Process.Pid {
		if pid > 0 && pid != child.Process.Pid {
			if p, findErr := os.FindProcess(pid); findErr == nil {
				_ = p.Kill()
				_ = p.Release()
			}
		}
		t.Fatalf("expected refusal for existing PID %d, got PID %d, error %v", child.Process.Pid, pid, err)
	}
}
func TestStopRefusesUnverifiedLegacyProcess(t *testing.T) {
	isolatedHome(t)
	child := helperProcess(t)
	path := legacyPID(t, child.Process.Pid)
	if err := StopDaemon(freeAddr(t)); err == nil {
		t.Fatal("stop claimed success for an unverified process")
	}
	if !IsProcessAlive(child.Process.Pid) {
		t.Fatal("unverified process was terminated")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("unverified live PID record removed")
	}
}
func TestIsolatedLifecycle(t *testing.T) {
	isolatedHome(t)
	addr := freeAddr(t)
	pid, err := StartDaemon(addr)
	if pid > 0 {
		t.Cleanup(func() { _ = StopDaemon(addr) })
	}
	if err != nil {
		t.Fatal(err)
	}
	if !IsProcessAlive(pid) {
		t.Fatal("started child not alive")
	}
	if healthy, got := GetStatus(addr); !healthy || got != pid {
		t.Fatalf("status = %v %d", healthy, got)
	}
	if healthy, got := GetStatus(freeAddr(t)); healthy || got != pid {
		t.Fatalf("wrong address status = %v %d", healthy, got)
	}
	if err := StopDaemon(addr); err != nil {
		t.Fatal(err)
	}
	if IsProcessAlive(pid) {
		t.Fatal("stop returned before process exited")
	}
	if healthy, got := GetStatus(addr); healthy || got != 0 {
		t.Fatalf("stopped status = %v %d", healthy, got)
	}
}
func TestInvalidAndStalePIDCleanup(t *testing.T) {
	for _, value := range []string{"0", "-1", "junk", "2147483647"} {
		t.Run(value, func(t *testing.T) {
			isolatedHome(t)
			path := legacyPID(t, 1)
			if err := os.WriteFile(path, []byte(value), 0600); err != nil {
				t.Fatal(err)
			}
			if h, p := GetStatus(freeAddr(t)); h || p != 0 {
				t.Fatalf("status = %v %d", h, p)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("invalid/stale record retained: %v", err)
			}
		})
	}
}
func TestEarlyChildExitUsefulFailure(t *testing.T) {
	isolatedHome(t)
	t.Setenv("VAL2HARNESS_DAEMON_TEST_EXIT", "1")
	// Pre-create directory so the baseline reaches process startup.
	path, _ := getPIDPath()
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	start := time.Now()
	pid, err := StartDaemon(freeAddr(t))
	if err == nil || !strings.Contains(err.Error(), "exit") {
		t.Fatalf("want child exit error, got PID %d, %v", pid, err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("child exit was hidden behind full health timeout")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("failed child left PID record")
	}
}

func TestPIDIdentityMismatchDoesNotKillProcess(t *testing.T) {
	isolatedHome(t)
	child := helperProcess(t)
	path := legacyPID(t, child.Process.Pid)
	// A saved creation marker from an earlier occupant of this numeric PID.
	data := []byte(fmt.Sprintf(`{"pid":%d,"identity":"previous-process","addr":"127.0.0.1:1"}`, child.Process.Pid))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := StopDaemon("127.0.0.1:1"); err == nil {
		t.Fatal("stop accepted a reused PID")
	}
	if !IsProcessAlive(child.Process.Pid) {
		t.Fatal("unrelated process killed")
	}
	if healthy, pid := GetStatus("127.0.0.1:1"); healthy || pid != 0 {
		t.Fatalf("status=%v %d", healthy, pid)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("stale identity record blocked recovery")
	}
}
func TestStartRejectsOccupiedAddress(t *testing.T) {
	isolatedHome(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	pid, err := StartDaemon(listener.Addr().String())
	if err == nil || pid != 0 {
		t.Fatalf("occupied address accepted: PID %d, %v", pid, err)
	}
}
func TestPIDPersistenceFailureStopsChild(t *testing.T) {
	isolatedHome(t)
	path, _ := getPIDPath()
	if err := os.MkdirAll(path+".val2harness.lock", 0700); err != nil {
		t.Fatal(err)
	}
	pid, err := StartDaemon(freeAddr(t))
	if err == nil || !strings.Contains(err.Error(), "persist") {
		t.Fatalf("want PID persistence error, got PID %d, %v", pid, err)
	}
	if pid <= 0 {
		t.Fatal("test did not exercise persistence after child launch")
	}
	if IsProcessAlive(pid) {
		t.Fatal("failed persistence left child alive")
	}
}
func TestDaemonDirectoryFailure(t *testing.T) {
	dir := isolatedHome(t)
	if err := os.WriteFile(filepath.Dir(dir), []byte("blocking file"), 0600); err != nil {
		t.Fatal(err)
	}
	if pid, err := StartDaemon(freeAddr(t)); err == nil || pid != 0 {
		t.Fatalf("want directory failure before spawn, got %d %v", pid, err)
	}
}
func TestConcurrentStartsCreateOneDaemon(t *testing.T) {
	isolatedHome(t)
	addr := freeAddr(t)
	type result struct {
		pid int
		err error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() { p, e := StartDaemon(addr); results <- result{p, e} }()
	}
	a, b := <-results, <-results
	t.Cleanup(func() { _ = StopDaemon(addr) })
	if a.pid <= 0 || a.pid != b.pid {
		t.Fatalf("concurrent starts created differing PIDs: %+v %+v", a, b)
	}
	if (a.err == nil) == (b.err == nil) {
		t.Fatalf("want exactly one successful start: %+v %+v", a, b)
	}
}

func TestStartRetiresReusedPIDWithoutTouchingUnrelatedProcess(t *testing.T) {
	isolatedHome(t)
	unrelated := helperProcess(t)
	path := legacyPID(t, unrelated.Process.Pid)
	data := []byte(fmt.Sprintf(`{"pid":%d,"identity":"earlier-occupant","addr":"127.0.0.1:1"}`, unrelated.Process.Pid))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	addr := freeAddr(t)
	pid, err := StartDaemon(addr)
	if err == nil {
		t.Cleanup(func() { _ = StopDaemon(addr) })
	}
	if err != nil {
		t.Fatalf("reused unrelated PID blocks recovery: %v", err)
	}
	if pid == unrelated.Process.Pid || !IsProcessAlive(unrelated.Process.Pid) {
		t.Fatal("recovery touched unrelated process")
	}
}
func TestStartRequiresChildControlHandshake(t *testing.T) {
	isolatedHome(t)
	t.Setenv("VAL2HARNESS_DAEMON_TEST_NO_CONTROL", "1")
	addr := freeAddr(t)
	pid, err := StartDaemon(addr)
	t.Cleanup(func() {
		path, _ := getPIDPath()
		record, _, readErr := readPID(path)
		if readErr == nil {
			_ = controlRequest(record, http.MethodPost)
			_ = waitForRecordedExit(record, 2*time.Second)
		}
	})
	if err == nil {
		t.Fatalf("public health alone accepted as child identity (PID %d)", pid)
	}
}
