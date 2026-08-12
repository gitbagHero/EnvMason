// Package profile defines and strictly normalizes EnvMason's platform-neutral,
// non-executable desired-capability declaration.
package profile

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

const SchemaVersion = "0.1.0"

const (
	ModuleBase         = "base"
	ModuleFrontendNode = "frontend_node"

	VariantMinimal  = "minimal"
	VariantStandard = "standard"

	StrategyLTS    = "lts"
	StrategyStable = "stable"
	StrategyExact  = "exact"
)

var profileNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

type Profile struct {
	SchemaVersion string   `json:"schema_version" yaml:"schema_version"`
	Name          string   `json:"name" yaml:"name"`
	Description   string   `json:"description,omitempty" yaml:"description,omitempty"`
	Modules       []Module `json:"modules" yaml:"modules"`
}

type Module struct {
	ID            string         `json:"id" yaml:"id"`
	Variant       string         `json:"variant,omitempty" yaml:"variant,omitempty"`
	VersionPolicy *VersionPolicy `json:"version_policy,omitempty" yaml:"version_policy,omitempty"`
	Options       *Options       `json:"options,omitempty" yaml:"options,omitempty"`
}

type VersionPolicy struct {
	Strategy string `json:"strategy" yaml:"strategy"`
	Pin      string `json:"pin,omitempty" yaml:"pin,omitempty"`
}

// Options is a closed union. Base and Frontend Node accept disjoint fields;
// Normalize rejects fields that do not belong to the selected module.
type Options struct {
	BuildTools            *bool `json:"build_tools,omitempty" yaml:"build_tools,omitempty"`
	TerminalConfiguration *bool `json:"terminal_configuration,omitempty" yaml:"terminal_configuration,omitempty"`
	Corepack              *bool `json:"corepack,omitempty" yaml:"corepack,omitempty"`
	PNPM                  *bool `json:"pnpm,omitempty" yaml:"pnpm,omitempty"`
	BrowserTesting        *bool `json:"browser_testing,omitempty" yaml:"browser_testing,omitempty"`
}

// Normalize validates a Profile, applies explicit defaults, and returns a deep
// copy with modules in canonical order. It never mutates its input.
func Normalize(value Profile) (Profile, error) {
	if err := validateProfileMetadata(value); err != nil {
		return Profile{}, err
	}
	if len(value.Modules) == 0 || len(value.Modules) > 2 {
		return Profile{}, errors.New("normalize profile: modules must contain one or two supported modules")
	}

	result := Profile{
		SchemaVersion: value.SchemaVersion,
		Name:          value.Name,
		Description:   value.Description,
		Modules:       make([]Module, 0, len(value.Modules)),
	}
	seen := make(map[string]string, len(value.Modules))
	for _, module := range value.Modules {
		variant := module.Variant
		if variant == "" {
			variant = VariantStandard
		}
		if previous, exists := seen[module.ID]; exists {
			if previous != variant {
				return Profile{}, fmt.Errorf(
					"normalize profile: module %q has conflicting variants %q and %q",
					module.ID, previous, variant,
				)
			}
			return Profile{}, fmt.Errorf("normalize profile: duplicate module %q", module.ID)
		}

		var normalized Module
		var err error
		switch module.ID {
		case ModuleBase:
			normalized, err = normalizeBase(module, variant)
		case ModuleFrontendNode:
			normalized, err = normalizeFrontendNode(module, variant)
		default:
			return Profile{}, fmt.Errorf("normalize profile: unsupported module %q", module.ID)
		}
		if err != nil {
			return Profile{}, err
		}
		seen[module.ID] = variant
		result.Modules = append(result.Modules, normalized)
	}

	sort.Slice(result.Modules, func(left, right int) bool {
		return moduleOrder(result.Modules[left].ID) < moduleOrder(result.Modules[right].ID)
	})
	return result, nil
}

func Validate(value Profile) error {
	_, err := Normalize(value)
	return err
}

func IsNormalized(value Profile) bool {
	normalized, err := Normalize(value)
	return err == nil && reflect.DeepEqual(value, normalized)
}

func validateProfileMetadata(value Profile) error {
	if value.SchemaVersion == "" {
		return fmt.Errorf(
			"normalize profile: schema_version is required; supported version is %q",
			SchemaVersion,
		)
	}
	if value.SchemaVersion != SchemaVersion {
		return fmt.Errorf(
			"normalize profile: unsupported schema_version %q; migration is unavailable; supported version is %q",
			value.SchemaVersion, SchemaVersion,
		)
	}
	if !profileNamePattern.MatchString(value.Name) {
		return errors.New("normalize profile: name must match ^[a-z][a-z0-9-]{0,62}$")
	}
	if value.Description != "" {
		if strings.TrimSpace(value.Description) != value.Description ||
			utf8.RuneCountInString(value.Description) > 256 {
			return errors.New("normalize profile: description must be 1-256 characters without surrounding whitespace")
		}
	}
	return nil
}

func normalizeBase(value Module, variant string) (Module, error) {
	if err := validateVariant(value.ID, variant); err != nil {
		return Module{}, err
	}
	if value.VersionPolicy != nil {
		return Module{}, errors.New("normalize profile: base does not accept version_policy")
	}
	if value.Options != nil &&
		(value.Options.Corepack != nil || value.Options.PNPM != nil || value.Options.BrowserTesting != nil) {
		return Module{}, errors.New("normalize profile: base contains Frontend Node options")
	}
	defaultValue := variant == VariantStandard
	options := &Options{
		BuildTools:            boolPointer(optionValue(value.Options, func(options *Options) *bool { return options.BuildTools }, defaultValue)),
		TerminalConfiguration: boolPointer(optionValue(value.Options, func(options *Options) *bool { return options.TerminalConfiguration }, defaultValue)),
	}
	return Module{ID: ModuleBase, Variant: variant, Options: options}, nil
}

func normalizeFrontendNode(value Module, variant string) (Module, error) {
	if err := validateVariant(value.ID, variant); err != nil {
		return Module{}, err
	}
	if value.Options != nil &&
		(value.Options.BuildTools != nil || value.Options.TerminalConfiguration != nil) {
		return Module{}, errors.New("normalize profile: frontend_node contains Base options")
	}
	policy, err := normalizeVersionPolicy(value.VersionPolicy)
	if err != nil {
		return Module{}, err
	}
	defaultValue := variant == VariantStandard
	options := &Options{
		Corepack:       boolPointer(optionValue(value.Options, func(options *Options) *bool { return options.Corepack }, defaultValue)),
		PNPM:           boolPointer(optionValue(value.Options, func(options *Options) *bool { return options.PNPM }, defaultValue)),
		BrowserTesting: boolPointer(optionValue(value.Options, func(options *Options) *bool { return options.BrowserTesting }, defaultValue)),
	}
	return Module{
		ID: ModuleFrontendNode, Variant: variant,
		VersionPolicy: policy, Options: options,
	}, nil
}

func validateVariant(module, variant string) error {
	if variant != VariantMinimal && variant != VariantStandard {
		return fmt.Errorf(
			"normalize profile: module %q has unsupported variant %q",
			module, variant,
		)
	}
	return nil
}

func normalizeVersionPolicy(value *VersionPolicy) (*VersionPolicy, error) {
	if value == nil {
		return &VersionPolicy{Strategy: StrategyLTS}, nil
	}
	switch value.Strategy {
	case StrategyLTS, StrategyStable:
		if value.Pin != "" {
			return nil, fmt.Errorf(
				"normalize profile: version strategy %q does not accept a pin",
				value.Strategy,
			)
		}
		return &VersionPolicy{Strategy: value.Strategy}, nil
	case StrategyExact:
		if !isExactStableVersion(value.Pin) {
			return nil, errors.New("normalize profile: exact version strategy requires a stable SemVer pin without a v prefix")
		}
		return &VersionPolicy{Strategy: StrategyExact, Pin: value.Pin}, nil
	default:
		return nil, fmt.Errorf(
			"normalize profile: unsupported version strategy %q",
			value.Strategy,
		)
	}
}

func isExactStableVersion(raw string) bool {
	parsed := versioncore.ParseSemVer(raw)
	return raw != "" && parsed.Comparable && parsed.Normalized == raw &&
		!strings.Contains(parsed.Normalized, "-") && !strings.Contains(parsed.Normalized, "+")
}

func optionValue(options *Options, field func(*Options) *bool, fallback bool) bool {
	if options == nil || field(options) == nil {
		return fallback
	}
	return *field(options)
}

func boolPointer(value bool) *bool {
	copy := value
	return &copy
}

func moduleOrder(id string) int {
	if id == ModuleBase {
		return 0
	}
	return 1
}
