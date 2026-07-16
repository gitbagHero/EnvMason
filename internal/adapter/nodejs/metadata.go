package nodejs

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const maxPackageJSONSize = 256 * 1024

type packageMetadata struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func metadataForExecutable(resolvedPath string) (packageMetadata, bool) {
	directory := filepath.Dir(resolvedPath)
	for depth := 0; depth < 8; depth++ {
		metadata, err := readPackageMetadata(filepath.Join(directory, "package.json"))
		if err == nil && metadata.Name != "" && metadata.Version != "" {
			return metadata, true
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	return packageMetadata{}, false
}

func readPackageMetadata(path string) (packageMetadata, error) {
	file, err := os.Open(path)
	if err != nil {
		return packageMetadata{}, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return packageMetadata{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxPackageJSONSize {
		return packageMetadata{}, errors.New("package metadata exceeds limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxPackageJSONSize+1))
	if err != nil {
		return packageMetadata{}, err
	}
	if len(data) > maxPackageJSONSize {
		return packageMetadata{}, errors.New("package metadata exceeds limit")
	}
	var metadata packageMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return packageMetadata{}, errors.New("invalid package metadata")
	}
	metadata.Name = strings.TrimSpace(metadata.Name)
	metadata.Version = strings.TrimSpace(metadata.Version)
	return metadata, nil
}

func isCorepackExecutable(resolvedPath string) bool {
	return strings.Contains(filepath.ToSlash(resolvedPath), "/node_modules/corepack/")
}

func metadataMatchesCommand(command string, metadata packageMetadata) bool {
	switch command {
	case "npm":
		return metadata.Name == "npm"
	case "corepack":
		return metadata.Name == "corepack"
	case "pnpm":
		return metadata.Name == "corepack" || metadata.Name == "pnpm" || metadata.Name == "@pnpm/exe"
	case "yarn":
		return metadata.Name == "corepack" || metadata.Name == "yarn" || metadata.Name == "@yarnpkg/cli-dist"
	default:
		return false
	}
}
