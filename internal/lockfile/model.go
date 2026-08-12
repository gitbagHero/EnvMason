// Package lockfile defines EnvMason's deterministic, non-executable
// single-target resolution record.
package lockfile

import (
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const SchemaVersion = "0.1.0"

type SourceKind string

const (
	SourcePackageCatalog  SourceKind = "package_catalog"
	SourceRuntimeCatalog  SourceKind = "runtime_catalog"
	SourcePlatformCatalog SourceKind = "platform_catalog"
)

type ResolutionState string

const (
	StateSatisfied       ResolutionState = "satisfied"
	StateInstallRequired ResolutionState = "install_required"
	StateConflict        ResolutionState = "conflict"
	StateUnresolved      ResolutionState = "unresolved"
)

const (
	ReasonCompatibleInstallation  = "compatible_installation_present"
	ReasonInstallationRequired    = "installation_required"
	ReasonConflictingInstallation = "conflicting_installation_present"
	ReasonVersionUnavailable      = "version_source_unavailable"
	ReasonUnsupportedPlatform     = "unsupported_platform"
	ReasonUnsupportedArchitecture = "unsupported_architecture"
	ReasonUnsupportedCapability   = "unsupported_capability"
)

type Lock struct {
	SchemaVersion string           `json:"schema_version"`
	ID            string           `json:"id"`
	GeneratedAt   time.Time        `json:"generated_at"`
	Executable    bool             `json:"executable"`
	Confirmable   bool             `json:"confirmable"`
	Profile       ProfileReference `json:"profile"`
	Target        Target           `json:"target"`
	Sources       []Source         `json:"sources"`
	Items         []Item           `json:"items"`
	Summary       Summary          `json:"summary"`
}

type ProfileReference struct {
	SchemaVersion string `json:"schema_version"`
	Name          string `json:"name"`
	Digest        string `json:"digest"`
}

type Target struct {
	OS           inventory.OperatingSystem `json:"os"`
	OSVersion    string                    `json:"os_version"`
	Architecture inventory.Architecture    `json:"architecture"`
}

type Source struct {
	ID         string     `json:"id"`
	Kind       SourceKind `json:"kind"`
	URI        string     `json:"uri"`
	SnapshotAt time.Time  `json:"snapshot_at"`
	Digest     string     `json:"digest"`
}

type Item struct {
	ID             string          `json:"id"`
	Module         string          `json:"module"`
	Capability     string          `json:"capability"`
	State          ResolutionState `json:"state"`
	Implementation *Implementation `json:"implementation,omitempty"`
	Observed       []Observation   `json:"observed"`
	Reason         string          `json:"reason"`
}

type Implementation struct {
	ToolID      string     `json:"tool_id"`
	Manager     string     `json:"manager"`
	PackageKind string     `json:"package_kind"`
	PackageID   string     `json:"package_id"`
	Version     string     `json:"version"`
	SourceID    string     `json:"source_id"`
	Conditions  Conditions `json:"conditions"`
}

type Conditions struct {
	OS            inventory.OperatingSystem `json:"os"`
	Architectures []inventory.Architecture  `json:"architectures"`
}

type Observation struct {
	Manager string `json:"manager"`
	Version string `json:"version"`
}

type Summary struct {
	Satisfied       int `json:"satisfied"`
	InstallRequired int `json:"install_required"`
	Conflict        int `json:"conflict"`
	Unresolved      int `json:"unresolved"`
}
