// Package workflow defines the immutable, non-executable contracts used to
// coordinate EnvMason's staged Node workflow. It does not generate Plans,
// accept confirmation or execute actions.
package workflow

import (
	"time"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

const (
	ManifestSchemaVersion = "0.1.0"
	RecordSchemaVersion   = "0.1.0"

	StageInstallNode     = "install_node"
	StageSetDefault      = "set_default"
	StageUpdateNodeTools = "update_node_tools"
)

const manifestSummary = "Install one exact NVM Node.js version, set it as the NVM default, then update selected Node tools through independently confirmed child Plans."

type Manifest struct {
	SchemaVersion     string          `json:"schema_version"`
	ID                string          `json:"id"`
	CreatedAt         time.Time       `json:"created_at"`
	Executable        bool            `json:"executable"`
	Confirmable       bool            `json:"confirmable"`
	Summary           string          `json:"summary"`
	TargetNodeVersion string          `json:"target_node_version"`
	NodeTools         NodeToolTargets `json:"node_tools"`
	Stages            []Stage         `json:"stages"`
}

type NodeToolTargets struct {
	NPM      string `json:"npm,omitempty"`
	Corepack string `json:"corepack,omitempty"`
	PNPM     string `json:"pnpm,omitempty"`
}

type Stage struct {
	ID                string    `json:"id"`
	Order             int       `json:"order"`
	Risk              plan.Risk `json:"risk"`
	PlanSchemaVersion string    `json:"plan_schema_version"`
	DependsOn         []string  `json:"depends_on"`
}

type BuildInput struct {
	CreatedAt         time.Time
	TargetNodeVersion string
	NodeTools         NodeToolTargets
}

type WorkflowState string

const (
	WorkflowPending     WorkflowState = "pending"
	WorkflowRunning     WorkflowState = "running"
	WorkflowCompleted   WorkflowState = "completed"
	WorkflowFailed      WorkflowState = "failed"
	WorkflowTimedOut    WorkflowState = "timed_out"
	WorkflowCancelled   WorkflowState = "cancelled"
	WorkflowInterrupted WorkflowState = "interrupted"
)

type StageState string

const (
	StagePending     StageState = "pending"
	StageReady       StageState = "ready"
	StagePlanned     StageState = "planned"
	StageRunning     StageState = "running"
	StageCompleted   StageState = "completed"
	StageFailed      StageState = "failed"
	StageTimedOut    StageState = "timed_out"
	StageCancelled   StageState = "cancelled"
	StageInterrupted StageState = "interrupted"
)

type Record struct {
	SchemaVersion string        `json:"schema_version"`
	ID            string        `json:"id"`
	ManifestID    string        `json:"manifest_id"`
	State         WorkflowState `json:"state"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	StartedAt     *time.Time    `json:"started_at,omitempty"`
	FinishedAt    *time.Time    `json:"finished_at,omitempty"`
	Stages        []StageRecord `json:"stages"`
	Transitions   []Transition  `json:"transitions"`
}

type StageRecord struct {
	ID                string     `json:"id"`
	State             StageState `json:"state"`
	PlanID            string     `json:"plan_id,omitempty"`
	PlanSchemaVersion string     `json:"plan_schema_version,omitempty"`
	OperationID       string     `json:"operation_id,omitempty"`
	CheckpointDigest  string     `json:"checkpoint_digest,omitempty"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	FinishedAt        *time.Time `json:"finished_at,omitempty"`
}

type Transition struct {
	StageID       string        `json:"stage_id"`
	From          StageState    `json:"from"`
	To            StageState    `json:"to"`
	WorkflowState WorkflowState `json:"workflow_state"`
	At            time.Time     `json:"at"`
	Reason        string        `json:"reason"`
}
