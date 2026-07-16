package nodejs

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const maxAliasSize = 4 * 1024

func installedNVMVersions(nvmDirectory string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(nvmDirectory, "versions", "node"))
	if err != nil {
		return nil, err
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() && validNodeVersion(entry.Name()) {
			versions = append(versions, entry.Name())
		}
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) < 0 })
	return versions, nil
}

func resolveDefaultAlias(nvmDirectory string, versions []string) (string, string, error) {
	value, err := readAlias(filepath.Join(nvmDirectory, "alias", "default"))
	if err != nil {
		return "", "", err
	}
	resolved, err := resolveAliasValue(nvmDirectory, value, versions, make(map[string]bool), 0)
	return value, resolved, err
}

func resolveAliasValue(nvmDirectory, value string, versions []string, visited map[string]bool, depth int) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || depth > 16 {
		return "", errors.New("invalid or recursive NVM alias")
	}
	if resolved := matchInstalledVersion(value, versions); resolved != "" {
		return resolved, nil
	}
	if value == "node" || value == "stable" {
		if len(versions) == 0 {
			return "", errors.New("NVM alias has no installed match")
		}
		return versions[len(versions)-1], nil
	}
	if !safeAliasName(value) || visited[value] {
		return "", errors.New("invalid or recursive NVM alias")
	}
	visited[value] = true
	next, err := readAlias(filepath.Join(nvmDirectory, "alias", filepath.FromSlash(value)))
	if err != nil {
		return "", errors.New("NVM alias target is unavailable")
	}
	return resolveAliasValue(nvmDirectory, next, versions, visited, depth+1)
}

func readAlias(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxAliasSize {
		return "", errors.New("NVM alias exceeds size limit")
	}
	data, err := io.ReadAll(io.LimitReader(file, maxAliasSize+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxAliasSize {
		return "", errors.New("NVM alias exceeds size limit")
	}
	return strings.TrimSpace(string(data)), nil
}

func safeAliasName(value string) bool {
	if filepath.IsAbs(value) || strings.Contains(value, "\\") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
		for _, character := range part {
			switch {
			case character >= 'a' && character <= 'z':
			case character >= 'A' && character <= 'Z':
			case character >= '0' && character <= '9':
			case strings.ContainsRune("*._-", character):
			default:
				return false
			}
		}
	}
	return true
}

func matchInstalledVersion(value string, versions []string) string {
	value = strings.TrimPrefix(value, "v")
	if value == "" {
		return ""
	}
	parts := strings.Split(value, ".")
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return ""
		}
	}
	for index := len(versions) - 1; index >= 0; index-- {
		candidate := strings.TrimPrefix(versions[index], "v")
		if candidate == value || strings.HasPrefix(candidate, value+".") {
			return versions[index]
		}
	}
	return ""
}

func validNodeVersion(value string) bool {
	parts := strings.Split(strings.TrimPrefix(value, "v"), ".")
	if len(parts) != 3 {
		return false
	}
	for _, part := range parts {
		if _, err := strconv.Atoi(part); err != nil {
			return false
		}
	}
	return strings.HasPrefix(value, "v")
}

func compareVersions(left, right string) int {
	l := strings.Split(strings.TrimPrefix(left, "v"), ".")
	r := strings.Split(strings.TrimPrefix(right, "v"), ".")
	for index := 0; index < 3; index++ {
		lv, _ := strconv.Atoi(l[index])
		rv, _ := strconv.Atoi(r[index])
		if lv < rv {
			return -1
		}
		if lv > rv {
			return 1
		}
	}
	return 0
}
