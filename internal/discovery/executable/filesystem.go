package executable

import (
	"io/fs"
	"os"
	"path/filepath"
)

type fileSystem interface {
	Lstat(string) (fs.FileInfo, error)
	Stat(string) (fs.FileInfo, error)
	EvalSymlinks(string) (string, error)
}

type osFileSystem struct{}

func (osFileSystem) Lstat(path string) (fs.FileInfo, error) {
	return os.Lstat(path)
}

func (osFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (osFileSystem) EvalSymlinks(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
