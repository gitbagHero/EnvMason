// Package recoverymanifest defines the immutable, review-only aggregation of
// already revalidated recovery candidates.
package recoverymanifest

import (
	"time"

	"github.com/gitbagHero/EnvMason/internal/execution"
)

const SchemaVersion = "0.1.0"

const manifestSummary = "Review-only recovery candidates and dispositions; this manifest cannot confirm, execute or restore anything."

type Disposition string

const (
	DispositionPrepareNewPlan       Disposition = "prepare_new_plan"
	DispositionManualAction         Disposition = "manual_action"
	DispositionInvestigateUncertain Disposition = "investigate_uncertain"
	DispositionReassessDrifted      Disposition = "reassess_drifted"
	DispositionReviewNoVerifier     Disposition = "review_without_verifier"
)

type Manifest struct {
	SchemaVersion string    `json:"schema_version"`
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	Executable    bool      `json:"executable"`
	Confirmable   bool      `json:"confirmable"`
	Summary       string    `json:"summary"`
	Sources       []Source  `json:"sources"`
	Items         []Item    `json:"items"`
}

type Source struct {
	OperationID string `json:"operation_id"`
	PlanID      string `json:"plan_id"`
}

type Item struct {
	SourceOperationID string                            `json:"source_operation_id"`
	SourcePlanID      string                            `json:"source_plan_id"`
	ActionID          string                            `json:"action_id"`
	ToolID            string                            `json:"tool_id"`
	Operation         string                            `json:"operation"`
	StepState         execution.State                   `json:"step_state"`
	Evidence          execution.RecoveryEvidence        `json:"evidence"`
	CurrentState      execution.RecoveryCheckpointState `json:"current_state"`
	RecoveryMode      string                            `json:"recovery_mode"`
	Disposition       Disposition                       `json:"disposition"`
	RecoverySummary   string                            `json:"recovery_summary"`
}
