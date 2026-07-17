package execution

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

func DefaultHistoryDirectory() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", errors.New("resolve operation history: user home is unavailable")
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "EnvMason", "operations"), nil
	case "windows":
		root := os.Getenv("LOCALAPPDATA")
		if root == "" {
			root = filepath.Join(home, "AppData", "Local")
		}
		return filepath.Join(root, "EnvMason", "operations"), nil
	default:
		root := os.Getenv("XDG_STATE_HOME")
		if root == "" {
			root = filepath.Join(home, ".local", "state")
		}
		return filepath.Join(root, "envmason", "operations"), nil
	}
}
