//go:build !windows

package execution

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureProcessTree(command *exec.Cmd, enabled bool) {
	if !enabled {
		return
	}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 5 * time.Second
}
