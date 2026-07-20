// Package plan defines and validates immutable EnvMason Plans. Plans contain
// declarative action identities only; executable paths and arguments belong to
// the controlled execution registry.
package plan

import "time"

const (
	SchemaVersion                   = "0.1.0"
	ExecutableSchemaVersion         = "0.2.0"
	HighRiskExecutableSchemaVersion = "0.3.0"
	DefaultTTL                      = 30 * time.Minute
)

type Risk string

const (
	RiskR0 Risk = "R0"
	RiskR1 Risk = "R1"
	RiskR2 Risk = "R2"
	RiskR3 Risk = "R3"
	RiskR4 Risk = "R4"
)

type Plan struct {
	SchemaVersion     string             `json:"schema_version"`
	ID                string             `json:"id"`
	CreatedAt         time.Time          `json:"created_at"`
	ExpiresAt         time.Time          `json:"expires_at"`
	Executable        bool               `json:"executable"`
	Summary           string             `json:"summary"`
	EnvironmentDigest string             `json:"environment_digest"`
	PolicyDigest      string             `json:"policy_digest"`
	Environment       EnvironmentSummary `json:"environment"`
	Actions           []Action           `json:"actions"`
}

type EnvironmentSummary struct {
	OS                   string                `json:"os"`
	OSVersion            string                `json:"os_version"`
	Architecture         string                `json:"architecture"`
	ToolID               string                `json:"tool_id"`
	ActiveInstallationID string                `json:"active_installation_id"`
	ActiveVersion        string                `json:"active_version"`
	ActiveManager        string                `json:"active_manager"`
	Installations        []InstallationSummary `json:"installations"`
}

type InstallationSummary struct {
	ID           string `json:"id"`
	Version      string `json:"version"`
	Path         string `json:"path"`
	Manager      string `json:"manager"`
	ActiveState  string `json:"active_state"`
	DefaultState string `json:"default_state"`
}

type Action struct {
	ID                string       `json:"id"`
	ToolID            string       `json:"tool_id"`
	Operation         string       `json:"operation"`
	Adapter           string       `json:"adapter"`
	TargetVersion     string       `json:"target_version"`
	Risk              Risk         `json:"risk"`
	Dependencies      []string     `json:"dependencies"`
	Confirmation      Confirmation `json:"confirmation"`
	ElevationRequired bool         `json:"elevation_required"`
	RestartRequired   bool         `json:"restart_required"`
	Download          Download     `json:"download"`
	Preconditions     []Check      `json:"preconditions"`
	Verifications     []Check      `json:"verifications"`
	Recovery          Recovery     `json:"recovery"`
}

type Confirmation struct {
	Required bool   `json:"required"`
	Scope    string `json:"scope"`
}

type Download struct {
	State string `json:"state"`
	Bytes *int64 `json:"bytes,omitempty"`
}

type Check struct {
	Kind     string `json:"kind"`
	Subject  string `json:"subject"`
	Expected string `json:"expected"`
}

type Recovery struct {
	Mode    string `json:"mode"`
	Summary string `json:"summary"`
}
