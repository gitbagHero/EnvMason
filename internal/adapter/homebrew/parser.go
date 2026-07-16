package homebrew

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

type infoDocument struct {
	Formulae []formulaRecord `json:"formulae"`
	Casks    []caskRecord    `json:"casks"`
}

type formulaRecord struct {
	Name      string                `json:"name"`
	FullName  string                `json:"full_name"`
	Tap       string                `json:"tap"`
	LinkedKeg string                `json:"linked_keg"`
	KegOnly   bool                  `json:"keg_only"`
	Installed []formulaInstallation `json:"installed"`
}

type formulaInstallation struct {
	Version               string `json:"version"`
	InstalledOnRequest    bool   `json:"installed_on_request"`
	InstalledAsDependency bool   `json:"installed_as_dependency"`
}

type caskRecord struct {
	Token     string           `json:"token"`
	FullToken string           `json:"full_token"`
	Tap       string           `json:"tap"`
	Installed stringList       `json:"installed"`
	Artifacts []map[string]any `json:"artifacts"`
}

type stringList []string

func (s *stringList) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		*s = nil
		return nil
	}
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*s = []string{single}
		return nil
	}
	var multiple []string
	if err := json.Unmarshal(data, &multiple); err != nil {
		return errors.New("installed version must be a string or string array")
	}
	*s = multiple
	return nil
}

type outdatedDocument struct {
	Formulae []outdatedRecord `json:"formulae"`
	Casks    []outdatedRecord `json:"casks"`
}

type outdatedRecord struct {
	Name              string   `json:"name"`
	InstalledVersions []string `json:"installed_versions"`
	CurrentVersion    string   `json:"current_version"`
	Pinned            bool     `json:"pinned"`
	PinnedVersion     *string  `json:"pinned_version"`
}

func parseInfo(data []byte, cellar, caskroom, home string, architecture inventory.Architecture, source inventory.SourceMetadata) ([]inventory.Tool, error) {
	var document infoDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode Homebrew info JSON v2: %w", err)
	}
	tools := make([]inventory.Tool, 0, len(document.Formulae)+len(document.Casks))
	for _, formula := range document.Formulae {
		name := formula.FullName
		if name == "" {
			name = formula.Name
		}
		if name == "" {
			continue
		}
		installations := make([]inventory.Installation, 0, len(formula.Installed))
		for _, installed := range formula.Installed {
			if installed.Version == "" {
				continue
			}
			activeState := inventory.ActiveStateInactive
			defaultState := inventory.DefaultStateNonDefault
			if formula.LinkedKeg == installed.Version {
				activeState = inventory.ActiveStateActive
				defaultState = inventory.DefaultStateDefault
			} else if formula.KegOnly {
				activeState = inventory.ActiveStateUnknown
				defaultState = inventory.DefaultStateUnknown
			}
			reason := inventory.InstallReasonDependency
			if installed.InstalledOnRequest {
				reason = inventory.InstallReasonDirect
			}
			path := "unknown"
			if cellar != "" {
				path = redactHome(pathpkg.Join(cellar, formula.Name, installed.Version), home)
			}
			installations = append(installations, inventory.Installation{
				ID:                "homebrew:formula:" + name + ":" + installed.Version,
				Version:           installed.Version,
				NormalizedVersion: installed.Version,
				Path:              path,
				Architecture:      architecture,
				Manager:           "homebrew",
				ActiveState:       activeState,
				DefaultState:      defaultState,
				InstallReason:     reason,
				Sources:           []inventory.SourceMetadata{source},
			})
		}
		if len(installations) == 0 {
			continue
		}
		tools = append(tools, inventory.Tool{
			ID:            toolID(PackageFormula, name),
			DisplayName:   name,
			Category:      inventory.CategoryUnknown,
			Installations: installations,
		})
	}
	for _, cask := range document.Casks {
		name := cask.FullToken
		if name == "" {
			name = cask.Token
		}
		if name == "" || len(cask.Installed) == 0 {
			continue
		}
		installations := make([]inventory.Installation, 0, len(cask.Installed))
		artifactTarget := firstArtifactTarget(cask.Artifacts)
		for _, version := range cask.Installed {
			if version == "" {
				continue
			}
			path := artifactTarget
			if path == "" && caskroom != "" {
				path = pathpkg.Join(caskroom, cask.Token, version)
			}
			if path == "" {
				path = "unknown"
			} else {
				path = redactHome(path, home)
			}
			installations = append(installations, inventory.Installation{
				ID:                "homebrew:cask:" + name + ":" + version,
				Version:           version,
				NormalizedVersion: version,
				Path:              path,
				Architecture:      architecture,
				Manager:           "homebrew",
				ActiveState:       inventory.ActiveStateUnknown,
				DefaultState:      inventory.DefaultStateUnknown,
				InstallReason:     inventory.InstallReasonUnknown,
				Sources:           []inventory.SourceMetadata{source},
			})
		}
		if len(installations) == 0 {
			continue
		}
		tools = append(tools, inventory.Tool{
			ID:            toolID(PackageCask, name),
			DisplayName:   name,
			Category:      inventory.CategoryUnknown,
			Installations: installations,
		})
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].ID < tools[j].ID })
	return tools, nil
}

func parseOutdated(data []byte) ([]OutdatedPackage, error) {
	var document outdatedDocument
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode Homebrew outdated JSON v2: %w", err)
	}
	result := make([]OutdatedPackage, 0, len(document.Formulae)+len(document.Casks))
	appendRecords := func(kind PackageKind, records []outdatedRecord) {
		for _, record := range records {
			if record.Name == "" {
				continue
			}
			pinnedVersion := ""
			if record.PinnedVersion != nil {
				pinnedVersion = *record.PinnedVersion
			}
			result = append(result, OutdatedPackage{
				Kind: kind, Name: record.Name,
				InstalledVersions: append([]string{}, record.InstalledVersions...),
				CurrentVersion:    record.CurrentVersion,
				Pinned:            record.Pinned, PinnedVersion: pinnedVersion,
			})
		}
	}
	appendRecords(PackageFormula, document.Formulae)
	appendRecords(PackageCask, document.Casks)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind == result[j].Kind {
			return result[i].Name < result[j].Name
		}
		return result[i].Kind < result[j].Kind
	})
	return result, nil
}

func firstArtifactTarget(artifacts []map[string]any) string {
	for _, artifact := range artifacts {
		if target, ok := artifact["target"].(string); ok && target != "" {
			return target
		}
	}
	return ""
}

func toolID(kind PackageKind, name string) string {
	parts := strings.FieldsFunc(strings.ToLower(name), func(character rune) bool { return character == '/' })
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		var builder strings.Builder
		for _, character := range part {
			switch {
			case character >= 'a' && character <= 'z', character >= '0' && character <= '9', character == '-', character == '_':
				builder.WriteRune(character)
			default:
				builder.WriteByte('-')
			}
		}
		value := strings.Trim(builder.String(), "-_")
		if value == "" || value[0] < 'a' || value[0] > 'z' {
			value = "pkg-" + value
		}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		normalized = []string{"unknown"}
	}
	return "homebrew." + string(kind) + "." + strings.Join(normalized, ".")
}
