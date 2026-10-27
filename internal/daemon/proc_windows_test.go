//go:build windows

package daemon

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsChildDoesNotOpenConsole(t *testing.T) {
	cmd := exec.Command("unused")
	setDaemonSysProcAttr(cmd)
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatal("background child can create a visible console")
	}
}
