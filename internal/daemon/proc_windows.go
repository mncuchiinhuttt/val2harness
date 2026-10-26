//go:build windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
)

const (
	createNewProcessGroup          = 0x00000200
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

func setDaemonSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup,
	}
}

func terminateProcess(proc *os.Process) {
	_ = proc.Kill()
}

func isProcessAlive(pid int) bool {
	h, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var code uint32
	if err := syscall.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}
