//go:build !windows

package daemon

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func setDaemonSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func terminateProcess(proc *os.Process) {
	_ = proc.Signal(syscall.SIGTERM)
	time.Sleep(500 * time.Millisecond)
	if IsProcessAlive(proc.Pid) {
		_ = proc.Signal(syscall.SIGKILL)
	}
}
