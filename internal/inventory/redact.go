package inventory

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// PathRedaction names one private absolute root and its stable Plan placeholder.
type PathRedaction struct {
	Root        string
	Placeholder string
}

// RedactInstallationPaths returns an Inventory copy whose installation paths
// use stable placeholders for private roots before the Inventory enters a Plan
// and becomes part of its immutable content ID.
func RedactInstallationPaths(value Inventory, redactions ...PathRedaction) (Inventory, error) {
	normalized := make([]PathRedaction, 0, len(redactions))
	for _, redaction := range redactions {
		if redaction.Root == "" {
			continue
		}
		if !filepath.IsAbs(redaction.Root) || redaction.Placeholder == "" ||
			strings.ContainsAny(redaction.Placeholder, `/\`) {
			return Inventory{}, errors.New("redact inventory paths: invalid private root or placeholder")
		}
		redaction.Root = filepath.Clean(redaction.Root)
		normalized = append(normalized, redaction)
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		return len(normalized[i].Root) > len(normalized[j].Root)
	})

	result := value
	result.Tools = append([]Tool{}, value.Tools...)
	for toolIndex := range result.Tools {
		result.Tools[toolIndex].Installations = append([]Installation{}, value.Tools[toolIndex].Installations...)
		for installationIndex := range result.Tools[toolIndex].Installations {
			path := result.Tools[toolIndex].Installations[installationIndex].Path
			result.Tools[toolIndex].Installations[installationIndex].Path = redactInstallationPath(path, normalized)
		}
	}
	return result, nil
}

func redactInstallationPath(path string, redactions []PathRedaction) string {
	for _, redaction := range redactions {
		relative, err := filepath.Rel(redaction.Root, filepath.Clean(path))
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			continue
		}
		if relative == "." {
			return redaction.Placeholder
		}
		return redaction.Placeholder + "/" + filepath.ToSlash(relative)
	}
	return path
}
