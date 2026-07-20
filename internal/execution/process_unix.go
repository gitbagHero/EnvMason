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
		// Re-signal briefly so a descendant forked concurrently with the first
		// group signal cannot escape after the leader has been selected to die.
		deadline := time.Now().Add(100 * time.Millisecond)
		killed := false
		for {
			err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
			switch {
			case err == nil:
				killed = true
			case errors.Is(err, syscall.ESRCH):
				if killed {
					return nil
				}
				return os.ErrProcessDone
			default:
				return err
			}
			if !time.Now().Before(deadline) {
				return nil
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	command.WaitDelay = 5 * time.Second
}
