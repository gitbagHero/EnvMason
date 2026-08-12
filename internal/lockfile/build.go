package lockfile

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/profile"
)

var (
	identifierPattern  = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)
	profileNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	packagePattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._+@/-]{0,127}$`)
	versionPattern     = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z.+_-]{0,127}$`)
	digestPattern      = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

type BuildInput struct {
	GeneratedAt time.Time
	Profile     profile.Profile
	Target      Target
	Sources     []Source
	Items       []Item
}

func Build(input BuildInput) (Lock, error) {
	if input.GeneratedAt.IsZero() {
		return Lock{}, errors.New("build Lock: generated_at is required")
	}
	profileData, err := profile.MarshalJSON(input.Profile)
	if err != nil {
		return Lock{}, fmt.Errorf("build Lock: invalid Profile: %w", err)
	}
	normalizedProfile, err := profile.Normalize(input.Profile)
	if err != nil {
		return Lock{}, fmt.Errorf("build Lock: invalid Profile: %w", err)
	}
	value := Lock{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   input.GeneratedAt.UTC(),
		Executable:    false,
		Confirmable:   false,
		Profile: ProfileReference{
			SchemaVersion: normalizedProfile.SchemaVersion,
			Name:          normalizedProfile.Name,
			Digest:        digest(profileData),
		},
		Target:  input.Target,
		Sources: cloneSources(input.Sources),
		Items:   cloneItems(input.Items),
	}
	canonicalize(&value)
	value.ID, err = calculateID(value)
	if err != nil {
		return Lock{}, err
	}
	if err := Validate(value); err != nil {
		return Lock{}, err
	}
	return value, nil
}

func Validate(value Lock) error {
	if value.SchemaVersion != SchemaVersion || value.GeneratedAt.IsZero() ||
		value.Executable || value.Confirmable || !digestPattern.MatchString(value.ID) {
		return errors.New("validate Lock: fixed identity, time or read-only flags are invalid")
	}
	if value.Profile.SchemaVersion != profile.SchemaVersion ||
		!profileNamePattern.MatchString(value.Profile.Name) ||
		!digestPattern.MatchString(value.Profile.Digest) {
		return errors.New("validate Lock: Profile reference is invalid")
	}
	if err := validateTarget(value.Target); err != nil {
		return err
	}
	if len(value.Sources) == 0 || len(value.Items) == 0 {
		return errors.New("validate Lock: sources and items are required")
	}
	sourceIDs := make(map[string]struct{}, len(value.Sources))
	for _, source := range value.Sources {
		if err := validateSource(source, value.GeneratedAt); err != nil {
			return err
		}
		if _, exists := sourceIDs[source.ID]; exists {
			return fmt.Errorf("validate Lock: duplicate source %q", source.ID)
		}
		sourceIDs[source.ID] = struct{}{}
	}
	itemIDs := make(map[string]struct{}, len(value.Items))
	capabilities := make(map[string]struct{}, len(value.Items))
	for _, item := range value.Items {
		if _, exists := itemIDs[item.ID]; exists {
			return fmt.Errorf("validate Lock: duplicate item %q", item.ID)
		}
		itemIDs[item.ID] = struct{}{}
		key := item.Module + "/" + item.Capability
		if _, exists := capabilities[key]; exists {
			return fmt.Errorf("validate Lock: duplicate capability %q", key)
		}
		capabilities[key] = struct{}{}
		if err := validateItem(item, value.Target, sourceIDs); err != nil {
			return err
		}
	}
	expected := cloneLock(value)
	canonicalize(&expected)
	if !reflect.DeepEqual(value, expected) {
		return errors.New("validate Lock: content is not canonically ordered or summary is invalid")
	}
	expectedID, err := calculateID(value)
	if err != nil || expectedID != value.ID {
		return errors.New("validate Lock: content-derived ID does not match")
	}
	return nil
}

func validateTarget(value Target) error {
	if value.OS != inventory.OSMacOS && value.OS != inventory.OSWindows && value.OS != inventory.OSLinux {
		return errors.New("validate Lock: target OS is unsupported")
	}
	if !versionPattern.MatchString(value.OSVersion) || !supportedArchitecture(value.Architecture) {
		return errors.New("validate Lock: target version or architecture is invalid")
	}
	return nil
}

func validateSource(value Source, generatedAt time.Time) error {
	if !identifierPattern.MatchString(value.ID) ||
		(value.Kind != SourcePackageCatalog && value.Kind != SourceRuntimeCatalog && value.Kind != SourcePlatformCatalog) ||
		value.SnapshotAt.IsZero() || value.SnapshotAt.After(generatedAt) || !digestPattern.MatchString(value.Digest) {
		return fmt.Errorf("validate Lock: source %q metadata is invalid", value.ID)
	}
	parsed, err := url.Parse(value.URI)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("validate Lock: source %q URI must be credential-free HTTPS without query or fragment", value.ID)
	}
	return nil
}

func validateItem(value Item, target Target, sources map[string]struct{}) error {
	if !identifierPattern.MatchString(value.ID) || !identifierPattern.MatchString(value.Capability) ||
		(value.Module != profile.ModuleBase && value.Module != profile.ModuleFrontendNode) {
		return fmt.Errorf("validate Lock: item %q identity is invalid", value.ID)
	}
	observations := make(map[string]struct{}, len(value.Observed))
	for _, observation := range value.Observed {
		if !validManager(observation.Manager, true) || !versionPattern.MatchString(observation.Version) {
			return fmt.Errorf("validate Lock: item %q observation is invalid", value.ID)
		}
		key := observation.Manager + "/" + observation.Version
		if _, exists := observations[key]; exists {
			return fmt.Errorf("validate Lock: item %q contains a duplicate observation", value.ID)
		}
		observations[key] = struct{}{}
	}
	if value.State == StateUnresolved {
		if value.Implementation != nil || !unresolvedReason(value.Reason) {
			return fmt.Errorf("validate Lock: unresolved item %q must omit implementation and use an unresolved reason", value.ID)
		}
		return nil
	}
	if value.Implementation == nil {
		return fmt.Errorf("validate Lock: resolved item %q requires an implementation", value.ID)
	}
	if err := validateImplementation(*value.Implementation, target, sources); err != nil {
		return fmt.Errorf("validate Lock: item %q: %w", value.ID, err)
	}
	matching := false
	for _, observation := range value.Observed {
		if observation.Manager == value.Implementation.Manager && observation.Version == value.Implementation.Version {
			matching = true
		}
	}
	switch value.State {
	case StateSatisfied:
		if value.Reason != ReasonCompatibleInstallation || !matching {
			return fmt.Errorf("validate Lock: satisfied item %q lacks a matching observation", value.ID)
		}
	case StateInstallRequired:
		if value.Reason != ReasonInstallationRequired || len(value.Observed) != 0 {
			return fmt.Errorf("validate Lock: install-required item %q has observations or wrong reason", value.ID)
		}
	case StateConflict:
		if value.Reason != ReasonConflictingInstallation || len(value.Observed) == 0 {
			return fmt.Errorf("validate Lock: conflict item %q requires observations", value.ID)
		}
	default:
		return fmt.Errorf("validate Lock: item %q state is unsupported", value.ID)
	}
	return nil
}

func validateImplementation(value Implementation, target Target, sources map[string]struct{}) error {
	if !identifierPattern.MatchString(value.ToolID) || !validManager(value.Manager, false) ||
		(value.PackageKind != "formula" && value.PackageKind != "runtime" && value.PackageKind != "system") ||
		!packagePattern.MatchString(value.PackageID) || !versionPattern.MatchString(value.Version) ||
		!identifierPattern.MatchString(value.SourceID) {
		return errors.New("implementation identity is invalid")
	}
	if _, exists := sources[value.SourceID]; !exists {
		return fmt.Errorf("implementation references unknown source %q", value.SourceID)
	}
	if value.Conditions.OS != target.OS || len(value.Conditions.Architectures) == 0 {
		return errors.New("implementation conditions do not match the Lock target")
	}
	matched := false
	seen := make(map[inventory.Architecture]struct{}, len(value.Conditions.Architectures))
	for _, architecture := range value.Conditions.Architectures {
		if !supportedArchitecture(architecture) {
			return errors.New("implementation contains an unsupported architecture")
		}
		if _, exists := seen[architecture]; exists {
			return errors.New("implementation contains a duplicate architecture")
		}
		seen[architecture] = struct{}{}
		if architecture == target.Architecture {
			matched = true
		}
	}
	if !matched {
		return errors.New("implementation conditions exclude the Lock target architecture")
	}
	return nil
}

func canonicalize(value *Lock) {
	value.GeneratedAt = value.GeneratedAt.UTC()
	for index := range value.Sources {
		value.Sources[index].SnapshotAt = value.Sources[index].SnapshotAt.UTC()
	}
	sort.Slice(value.Sources, func(left, right int) bool { return value.Sources[left].ID < value.Sources[right].ID })
	for index := range value.Items {
		item := &value.Items[index]
		sort.Slice(item.Observed, func(left, right int) bool {
			if item.Observed[left].Manager != item.Observed[right].Manager {
				return item.Observed[left].Manager < item.Observed[right].Manager
			}
			return item.Observed[left].Version < item.Observed[right].Version
		})
		if item.Implementation != nil {
			sort.Slice(item.Implementation.Conditions.Architectures, func(left, right int) bool {
				return item.Implementation.Conditions.Architectures[left] < item.Implementation.Conditions.Architectures[right]
			})
		}
	}
	sort.Slice(value.Items, func(left, right int) bool { return value.Items[left].ID < value.Items[right].ID })
	value.Summary = Summary{}
	for _, item := range value.Items {
		switch item.State {
		case StateSatisfied:
			value.Summary.Satisfied++
		case StateInstallRequired:
			value.Summary.InstallRequired++
		case StateConflict:
			value.Summary.Conflict++
		case StateUnresolved:
			value.Summary.Unresolved++
		}
	}
}

func calculateID(value Lock) (string, error) {
	copy := cloneLock(value)
	copy.ID = ""
	data, err := json.Marshal(copy)
	if err != nil {
		return "", errors.New("calculate Lock ID: encode content")
	}
	return digest(data), nil
}

func digest(data []byte) string {
	value := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(value[:])
}

func cloneLock(value Lock) Lock {
	result := value
	result.Sources = cloneSources(value.Sources)
	result.Items = cloneItems(value.Items)
	return result
}

func cloneSources(values []Source) []Source { return append([]Source{}, values...) }

func cloneItems(values []Item) []Item {
	result := make([]Item, len(values))
	for index, value := range values {
		result[index] = value
		result[index].Observed = append([]Observation{}, value.Observed...)
		if value.Implementation != nil {
			implementation := *value.Implementation
			implementation.Conditions.Architectures = append([]inventory.Architecture{}, value.Implementation.Conditions.Architectures...)
			result[index].Implementation = &implementation
		}
	}
	return result
}

func supportedArchitecture(value inventory.Architecture) bool {
	switch value {
	case inventory.ArchitectureAMD64, inventory.ArchitectureARM64, inventory.Architecture386,
		inventory.ArchitectureARM, inventory.ArchitecturePPC64, inventory.ArchitecturePPC64LE,
		inventory.ArchitectureS390X, inventory.ArchitectureRISCV64:
		return true
	default:
		return false
	}
}

func validManager(value string, observed bool) bool {
	return value == "homebrew" || value == "nvm" || value == "system" || (observed && value == "unknown")
}

func unresolvedReason(value string) bool {
	return value == ReasonVersionUnavailable || value == ReasonUnsupportedPlatform ||
		value == ReasonUnsupportedArchitecture || value == ReasonUnsupportedCapability
}
