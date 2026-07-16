package inventory

import "time"

const (
	SchemaVersion       = "0.2.0"
	LegacySchemaVersion = "0.1.0"
)

type Architecture string

const (
	ArchitectureAMD64   Architecture = "amd64"
	ArchitectureARM64   Architecture = "arm64"
	Architecture386     Architecture = "386"
	ArchitectureARM     Architecture = "arm"
	ArchitecturePPC64   Architecture = "ppc64"
	ArchitecturePPC64LE Architecture = "ppc64le"
	ArchitectureS390X   Architecture = "s390x"
	ArchitectureRISCV64 Architecture = "riscv64"
	ArchitectureUnknown Architecture = "unknown"
)

type OperatingSystem string

const (
	OSMacOS   OperatingSystem = "macos"
	OSWindows OperatingSystem = "windows"
	OSLinux   OperatingSystem = "linux"
	OSUnknown OperatingSystem = "unknown"
)

type Confidence string

const (
	ConfidenceHigh    Confidence = "high"
	ConfidenceMedium  Confidence = "medium"
	ConfidenceLow     Confidence = "low"
	ConfidenceUnknown Confidence = "unknown"
)

type SourceKind string

const (
	SourceCommand        SourceKind = "command"
	SourceFile           SourceKind = "file"
	SourcePackageManager SourceKind = "package_manager"
	SourceEnvironment    SourceKind = "environment"
	SourceManual         SourceKind = "manual"
	SourceFixture        SourceKind = "fixture"
	SourceUnknown        SourceKind = "unknown"
)

type ToolCategory string

const (
	CategoryBase      ToolCategory = "base"
	CategoryRuntime   ToolCategory = "runtime"
	CategoryEcosystem ToolCategory = "ecosystem"
	CategoryContainer ToolCategory = "container"
	CategorySDK       ToolCategory = "sdk"
	CategoryDevOps    ToolCategory = "devops"
	CategoryAgent     ToolCategory = "agent"
	CategoryUnknown   ToolCategory = "unknown"
)

type ActiveState string

const (
	ActiveStateActive   ActiveState = "active"
	ActiveStateInactive ActiveState = "inactive"
	ActiveStateUnknown  ActiveState = "unknown"
)

type DefaultState string

const (
	DefaultStateDefault    DefaultState = "default"
	DefaultStateNonDefault DefaultState = "non_default"
	DefaultStateUnknown    DefaultState = "unknown"
)

type InstallReason string

const (
	InstallReasonDirect     InstallReason = "direct"
	InstallReasonDependency InstallReason = "dependency"
	InstallReasonUnknown    InstallReason = "unknown"
)

type FindingSeverity string

const (
	SeverityInfo    FindingSeverity = "info"
	SeverityWarning FindingSeverity = "warning"
	SeverityError   FindingSeverity = "error"
)

type TranslationState string

const (
	TranslationStateNative     TranslationState = "native"
	TranslationStateTranslated TranslationState = "translated"
	TranslationStateUnknown    TranslationState = "unknown"
)

type PathState string

const (
	PathStateExists  PathState = "exists"
	PathStateMissing PathState = "missing"
	PathStateUnknown PathState = "unknown"
)

type Inventory struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   time.Time `json:"generated_at"`
	System        System    `json:"system"`
	Tools         []Tool    `json:"tools"`
	Findings      []Finding `json:"findings"`
}

type System struct {
	OS                  OperatingSystem  `json:"os"`
	OSVersion           string           `json:"os_version"`
	OSBuild             string           `json:"os_build"`
	Architecture        Architecture     `json:"architecture"`
	ProcessArchitecture Architecture     `json:"process_architecture"`
	TranslationState    TranslationState `json:"translation_state"`
	Shell               Shell            `json:"shell"`
	PathEntries         []PathEntry      `json:"path_entries"`
	Sources             []SourceMetadata `json:"sources"`
}

type Shell struct {
	LoginPath    string `json:"login_path"`
	LoginName    string `json:"login_name"`
	InvokingPath string `json:"invoking_path"`
	InvokingName string `json:"invoking_name"`
}

type PathEntry struct {
	Position  int       `json:"position"`
	Value     string    `json:"value"`
	State     PathState `json:"state"`
	Duplicate bool      `json:"duplicate"`
}

type Tool struct {
	ID            string         `json:"id"`
	DisplayName   string         `json:"display_name"`
	Category      ToolCategory   `json:"category"`
	Installations []Installation `json:"installations"`
}

type Installation struct {
	ID                string           `json:"id"`
	Version           string           `json:"version"`
	NormalizedVersion string           `json:"normalized_version,omitempty"`
	Path              string           `json:"path"`
	Architecture      Architecture     `json:"architecture"`
	Manager           string           `json:"manager"`
	ActiveState       ActiveState      `json:"active_state"`
	DefaultState      DefaultState     `json:"default_state"`
	InstallReason     InstallReason    `json:"install_reason"`
	Sources           []SourceMetadata `json:"sources"`
}

type Finding struct {
	ID             string           `json:"id"`
	Code           string           `json:"code"`
	Severity       FindingSeverity  `json:"severity"`
	Message        string           `json:"message"`
	Evidence       []string         `json:"evidence"`
	Confidence     Confidence       `json:"confidence"`
	ToolID         string           `json:"tool_id,omitempty"`
	InstallationID string           `json:"installation_id,omitempty"`
	Sources        []SourceMetadata `json:"sources"`
}

type SourceMetadata struct {
	Kind        SourceKind `json:"kind"`
	Name        string     `json:"name"`
	CollectedAt time.Time  `json:"collected_at"`
	Confidence  Confidence `json:"confidence"`
}
