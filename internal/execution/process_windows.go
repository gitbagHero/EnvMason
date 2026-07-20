//go:build windows

package execution

import "os/exec"

func configureProcessTree(_ *exec.Cmd, _ bool) {}
