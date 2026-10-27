//go:build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

func setDaemonSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
}
func IsProcessAlive(pid int) bool {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return false
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	// Access denied is not evidence that a process is dead.
	if err == windows.ERROR_ACCESS_DENIED {
		return true
	}
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	state, err := windows.WaitForSingleObject(handle, 0)
	return err == nil && state == uint32(windows.WAIT_TIMEOUT)
}
func handleIdentity(handle windows.Handle) (string, error) {
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return "", err
	}
	name := make([]uint16, 32768)
	size := uint32(len(name))
	if err := windows.QueryFullProcessImageName(handle, 0, &name[0], &size); err != nil {
		return "", err
	}
	return strconv.FormatUint(uint64(creation.HighDateTime)<<32|uint64(creation.LowDateTime), 10) + ":" + strings.ToLower(windows.UTF16ToString(name[:size])), nil
}
func processIdentity(pid int) (string, error) {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(handle)
	return handleIdentity(handle)
}
func terminateProcess(pid int, identity string) error {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	actual, err := handleIdentity(handle)
	if err != nil {
		return err
	}
	if actual != identity {
		return fmt.Errorf("process identity changed; refusing to terminate an unrelated process")
	}
	// Both verification and termination use the same handle, so PID reuse cannot
	// redirect the kill to a different process after verification.
	if err := windows.TerminateProcess(handle, 1); err != nil {
		return err
	}
	state, err := windows.WaitForSingleObject(handle, 3000)
	if err != nil {
		return err
	}
	if state != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("process has not exited after termination")
	}
	return nil
}
func lockFile(file *os.File) error {
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &windows.Overlapped{})
}
func unlockFile(file *os.File) {
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &windows.Overlapped{})
}
