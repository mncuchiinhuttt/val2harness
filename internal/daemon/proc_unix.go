//go:build !windows

package daemon

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
)

func setDaemonSysProcAttr(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	defer process.Release()
	err = process.Signal(syscall.Signal(0))
	if err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	if runtime.GOOS == "linux" {
		if data, e := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid)); e == nil {
			fields := strings.Fields(string(data)[strings.LastIndex(string(data), ")")+1:])
			if len(fields) > 0 && (fields[0] == "Z" || fields[0] == "X") {
				return false
			}
		}
	}
	return true
}
func processIdentity(pid int) (string, error) {
	if runtime.GOOS == "linux" {
		data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return "", err
		}
		fields := strings.Fields(string(data)[strings.LastIndex(string(data), ")")+1:])
		if len(fields) < 20 {
			return "", fmt.Errorf("cannot read process start time")
		}
		executable, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
		if err != nil {
			return "", err
		}
		return fields[19] + ":" + executable, nil
	}
	// macOS/BSD expose process start time and executable through their native ps.
	cmd := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=", "-o", "comm=")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	data, err := cmd.Output()
	if err != nil {
		return "", err
	}
	identity := strings.TrimSpace(string(data))
	if identity == "" {
		return "", fmt.Errorf("cannot identify process")
	}
	return identity, nil
}
func terminateProcess(pid int, identity string) error {
	return fmt.Errorf("authenticated graceful control is unavailable; stop the original gateway manually (numeric PID signaling cannot safely exclude PID reuse on this platform)")
}
func lockFile(file *os.File) error { return syscall.Flock(int(file.Fd()), syscall.LOCK_EX) }
func unlockFile(file *os.File)     { _ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN) }
