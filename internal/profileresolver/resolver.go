// Package profileresolver maps a normalized Profile and an explicit catalog
// snapshot to a deterministic, non-executable Lock. It performs no discovery,
// network access, package-manager calls or writes.
package profileresolver

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/profile"
	versioncore "github.com/gitbagHero/EnvMason/internal/version"
)

const (
	ChannelStable = "stable"
	ChannelLTS    = "lts"

	CapabilityBaseGit      = "base.git"
	CapabilityBaseSSH      = "base.ssh"
	CapabilityBaseCMake    = "base.cmake"
	CapabilityBaseTerminal = "base.terminal"

	CapabilityFrontendNode           = "frontend.node"
	CapabilityFrontendNPM            = "frontend.npm"
	CapabilityFrontendCorepack       = "frontend.corepack"
	CapabilityFrontendPNPM           = "frontend.pnpm"
	CapabilityFrontendBrowserTesting = "frontend.browser_testing"
)

type Catalog struct {
	Sources []lockfile.Source
	Entries []CatalogEntry
}

type CatalogEntry struct {
	Capability     string
	Channel        string
	Implementation lockfile.Implementation
}

type Input struct {
	GeneratedAt time.Time
	Profile     profile.Profile
	Inventory   inventory.Inventory
	Target      lockfile.Target
	Catalog     Catalog
}

type desiredCapability struct {
	module, id, channel, exact string
}

func Resolve(input Input) (lockfile.Lock, error) {
	if input.GeneratedAt.IsZero() {
		return lockfile.Lock{}, errors.New("resolve Profile: generated_at is required")
	}
	normalized, err := profile.Normalize(input.Profile)
	if err != nil {
		return lockfile.Lock{}, fmt.Errorf("resolve Profile: %w", err)
	}
	if err := validateInventoryTarget(input.Inventory, input.Target, input.GeneratedAt); err != nil {
		return lockfile.Lock{}, err
	}
	if len(input.Catalog.Sources) == 0 {
		return lockfile.Lock{}, errors.New("resolve Profile: catalog requires at least one source snapshot")
	}
	if err := validateCatalog(input.Catalog); err != nil {
		return lockfile.Lock{}, err
	}

	desired := desiredCapabilities(normalized)
	items := make([]lockfile.Item, 0, len(desired))
	for _, capability := range desired {
		items = append(items, resolveCapability(capability, input.Target, input.Catalog, input.Inventory))
	}
	return lockfile.Build(lockfile.BuildInput{
		GeneratedAt: input.GeneratedAt,
		Profile:     normalized,
		Target:      input.Target,
		Sources:     input.Catalog.Sources,
		Items:       items,
	})
}

func validateInventoryTarget(value inventory.Inventory, target lockfile.Target, generatedAt time.Time) error {
	if value.SchemaVersion != inventory.SchemaVersion || value.GeneratedAt.IsZero() || value.GeneratedAt.After(generatedAt) {
		return errors.New("resolve Profile: current Inventory is required")
	}
	if value.System.OS != target.OS || value.System.OSVersion != target.OSVersion ||
		value.System.Architecture != target.Architecture {
		return errors.New("resolve Profile: Inventory system does not match the Lock target")
	}
	return nil
}

func validateCatalog(value Catalog) error {
	sources := make(map[string]struct{}, len(value.Sources))
	for _, source := range value.Sources {
		if _, exists := sources[source.ID]; exists {
			return fmt.Errorf("resolve Profile: duplicate catalog source %q", source.ID)
		}
		sources[source.ID] = struct{}{}
	}
	for _, entry := range value.Entries {
		if !knownCapability(entry.Capability) {
			return fmt.Errorf("resolve Profile: catalog contains unsupported capability %q", entry.Capability)
		}
		if entry.Channel != ChannelStable && entry.Channel != ChannelLTS {
			return fmt.Errorf("resolve Profile: catalog capability %q has unsupported channel %q", entry.Capability, entry.Channel)
		}
		if _, exists := sources[entry.Implementation.SourceID]; !exists {
			return fmt.Errorf(
				"resolve Profile: catalog capability %q references unknown source %q",
				entry.Capability, entry.Implementation.SourceID,
			)
		}
	}
	return nil
}

func desiredCapabilities(value profile.Profile) []desiredCapability {
	result := []desiredCapability{}
	for _, module := range value.Modules {
		switch module.ID {
		case profile.ModuleBase:
			result = append(result,
				desiredCapability{module: module.ID, id: CapabilityBaseGit, channel: ChannelStable},
				desiredCapability{module: module.ID, id: CapabilityBaseSSH, channel: ChannelStable},
			)
			if *module.Options.BuildTools {
				result = append(result, desiredCapability{module: module.ID, id: CapabilityBaseCMake, channel: ChannelStable})
			}
			if *module.Options.TerminalConfiguration {
				result = append(result, desiredCapability{module: module.ID, id: CapabilityBaseTerminal, channel: ChannelStable})
			}
		case profile.ModuleFrontendNode:
			node := desiredCapability{module: module.ID, id: CapabilityFrontendNode}
			if module.VersionPolicy.Strategy == profile.StrategyExact {
				node.exact = module.VersionPolicy.Pin
			} else {
				node.channel = module.VersionPolicy.Strategy
			}
			result = append(result, node,
				desiredCapability{module: module.ID, id: CapabilityFrontendNPM, channel: ChannelStable},
			)
			if *module.Options.Corepack {
				result = append(result, desiredCapability{module: module.ID, id: CapabilityFrontendCorepack, channel: ChannelStable})
			}
			if *module.Options.PNPM {
				result = append(result, desiredCapability{module: module.ID, id: CapabilityFrontendPNPM, channel: ChannelStable})
			}
			if *module.Options.BrowserTesting {
				result = append(result, desiredCapability{module: module.ID, id: CapabilityFrontendBrowserTesting, channel: ChannelStable})
			}
		}
	}
	return result
}

func resolveCapability(
	desired desiredCapability,
	target lockfile.Target,
	catalog Catalog,
	current inventory.Inventory,
) lockfile.Item {
	item := lockfile.Item{
		ID: desired.id, Module: desired.module, Capability: desired.id,
		State: lockfile.StateUnresolved, Observed: []lockfile.Observation{},
	}
	switch {
	case target.OS != inventory.OSMacOS:
		item.Reason = lockfile.ReasonUnsupportedPlatform
		return item
	case target.Architecture != inventory.ArchitectureARM64 && target.Architecture != inventory.ArchitectureAMD64:
		item.Reason = lockfile.ReasonUnsupportedArchitecture
		return item
	case desired.id == CapabilityBaseTerminal || desired.id == CapabilityFrontendBrowserTesting:
		item.Reason = lockfile.ReasonUnsupportedCapability
		return item
	}

	candidates := catalogCandidates(desired, target, catalog.Entries)
	if len(candidates) != 1 {
		item.Reason = lockfile.ReasonVersionUnavailable
		return item
	}
	implementation := cloneImplementation(candidates[0].Implementation)
	item.Implementation = &implementation
	item.Observed, item.State = observedState(current, implementation, target.Architecture)
	switch item.State {
	case lockfile.StateSatisfied:
		item.Reason = lockfile.ReasonCompatibleInstallation
	case lockfile.StateInstallRequired:
		item.Reason = lockfile.ReasonInstallationRequired
	case lockfile.StateConflict:
		item.Reason = lockfile.ReasonConflictingInstallation
	}
	return item
}

func catalogCandidates(desired desiredCapability, target lockfile.Target, entries []CatalogEntry) []CatalogEntry {
	result := []CatalogEntry{}
	for _, entry := range entries {
		if entry.Capability != desired.id || entry.Implementation.Conditions.OS != target.OS ||
			!containsArchitecture(entry.Implementation.Conditions.Architectures, target.Architecture) {
			continue
		}
		if desired.exact != "" {
			if !sameVersion(entry.Implementation.Version, desired.exact) {
				continue
			}
		} else if entry.Channel != desired.channel {
			continue
		}
		result = append(result, entry)
	}
	return result
}

func observedState(
	current inventory.Inventory,
	implementation lockfile.Implementation,
	targetArchitecture inventory.Architecture,
) ([]lockfile.Observation, lockfile.ResolutionState) {
	observed := []lockfile.Observation{}
	seen := map[string]struct{}{}
	matching := false
	for _, tool := range current.Tools {
		if tool.ID != implementation.ToolID {
			continue
		}
		for _, installation := range tool.Installations {
			version := normalizedVersion(installation)
			manager := installation.Manager
			if !knownObservedManager(manager) {
				manager = "unknown"
			}
			if version == "" {
				version = "unknown"
			}
			key := manager + "/" + version
			if _, exists := seen[key]; !exists {
				seen[key] = struct{}{}
				observed = append(observed, lockfile.Observation{Manager: manager, Version: version})
			}
			architectureMatches := installation.Architecture == inventory.ArchitectureUnknown ||
				installation.Architecture == "" || installation.Architecture == targetArchitecture
			if manager == implementation.Manager && sameVersion(version, implementation.Version) && architectureMatches {
				matching = true
			}
		}
	}
	sort.Slice(observed, func(left, right int) bool {
		if observed[left].Manager != observed[right].Manager {
			return observed[left].Manager < observed[right].Manager
		}
		return observed[left].Version < observed[right].Version
	})
	switch {
	case matching:
		return observed, lockfile.StateSatisfied
	case len(observed) == 0:
		return observed, lockfile.StateInstallRequired
	default:
		return observed, lockfile.StateConflict
	}
}

func normalizedVersion(value inventory.Installation) string {
	raw := value.NormalizedVersion
	if raw == "" {
		raw = value.Version
	}
	if raw == "" {
		return ""
	}
	parsed := versioncore.ParseSemVer(raw)
	if parsed.Comparable {
		return parsed.Normalized
	}
	if safeVersion(raw) {
		return raw
	}
	return ""
}

func sameVersion(left, right string) bool {
	leftValue := versioncore.ParseSemVer(left)
	rightValue := versioncore.ParseSemVer(right)
	if leftValue.Comparable && rightValue.Comparable {
		return leftValue.Normalized == rightValue.Normalized
	}
	return left == right
}

func safeVersion(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for index, character := range value {
		if index == 0 && !asciiAlphaNumeric(character) {
			return false
		}
		if !asciiAlphaNumeric(character) && !strings.ContainsRune(".+_-", character) {
			return false
		}
	}
	return true
}

func asciiAlphaNumeric(value rune) bool {
	return value >= '0' && value <= '9' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z'
}

func containsArchitecture(values []inventory.Architecture, target inventory.Architecture) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneImplementation(value lockfile.Implementation) lockfile.Implementation {
	result := value
	result.Conditions.Architectures = append([]inventory.Architecture{}, value.Conditions.Architectures...)
	return result
}

func knownObservedManager(value string) bool {
	return value == "homebrew" || value == "nvm" || value == "system" || value == "unknown"
}

func knownCapability(value string) bool {
	switch value {
	case CapabilityBaseGit, CapabilityBaseSSH, CapabilityBaseCMake, CapabilityBaseTerminal,
		CapabilityFrontendNode, CapabilityFrontendNPM, CapabilityFrontendCorepack,
		CapabilityFrontendPNPM, CapabilityFrontendBrowserTesting:
		return true
	default:
		return false
	}
}
