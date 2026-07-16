//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package executable

import (
	"errors"
	"syscall"
)

func isPlatformSymlinkLoop(err error) bool {
	return errors.Is(err, syscall.ELOOP)
}
