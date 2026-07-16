//go:build windows

package executable

func isPlatformSymlinkLoop(error) bool {
	return false
}
