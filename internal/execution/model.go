// Package execution provides the deterministic, registry-backed EnvMason
// executor and its redacted local operation records.
package execution

import (
	"context"
	"time"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

const (
	RecordSchemaVersion = "0.1.0"
	DefaultOutputLimit  = 64 << 10
	MaximumTimeout      = 30 * time.Second
)

type State string

const (
	StatePending     State = "pending"
	StateRunning     State = "running"
	StateVerifying   State = "verifying"
	StateCompleted   State = "completed"
	StateFailed      State = "failed"
	StateTimedOut    State = "timed_out"
	StateCancelled   State = "cancelled"
	StateInterrupted State = "interrupted"
)

type CheckState string

const (
	CheckPending CheckState = "pending"
	CheckPassed  CheckState = "passed"
	CheckFailed  CheckState = "failed"
	CheckNotRun  CheckState = "not_run"
)

type ConfirmationReceipt struct {
	Scope           string    `json:"scope"`
	ConfirmedPlanID string    `json:"confirmed_plan_id"`
	ConfirmedAt     time.Time `json:"confirmed_at"`
}

type Request struct {
	Plan            plan.Plan
	Confirmation    ConfirmationReceipt
	SensitiveValues []string
}

type Record struct {
	SchemaVersion     string              `json:"schema_version"`
	ID                string              `json:"id"`
	PlanID            string              `json:"plan_id"`
	PlanSchemaVersion string              `json:"plan_schema_version"`
	State             State               `json:"state"`
	CreatedAt         time.Time           `json:"created_at"`
	UpdatedAt         time.Time           `json:"updated_at"`
	StartedAt         *time.Time          `json:"started_at,omitempty"`
	FinishedAt        *time.Time          `json:"finished_at,omitempty"`
	Confirmation      ConfirmationReceipt `json:"confirmation"`
	Steps             []StepRecord        `json:"steps"`
	Transitions       []Transition        `json:"transitions"`
}

type StepRecord struct {
	ActionID     string         `json:"action_id"`
	ToolID       string         `json:"tool_id"`
	Operation    string         `json:"operation"`
	Adapter      string         `json:"adapter"`
	Risk         plan.Risk      `json:"risk"`
	State        State          `json:"state"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	FinishedAt   *time.Time     `json:"finished_at,omitempty"`
	Invocation   *Invocation    `json:"invocation,omitempty"`
	ExitCode     *int           `json:"exit_code,omitempty"`
	Error        *ErrorDetail   `json:"error,omitempty"`
	Precondition CheckResult    `json:"precondition"`
	Verification CheckResult    `json:"verification"`
	Stdout       CapturedOutput `json:"stdout"`
	Stderr       CapturedOutput `json:"stderr"`
}

type Invocation struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
}

type ErrorDetail struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

type CheckResult struct {
	State   CheckState `json:"state"`
	Message string     `json:"message,omitempty"`
}

type CapturedOutput struct {
	Text      string `json:"text"`
	Truncated bool   `json:"truncated"`
}

type Transition struct {
	State    State     `json:"state"`
	At       time.Time `json:"at"`
	ActionID string    `json:"action_id,omitempty"`
	Reason   string    `json:"reason"`
}

type CommandSpec struct {
	Executable      string
	Args            []string
	Environment     []string
	Directory       string
	Timeout         time.Duration
	SensitiveValues []string
}

type ProcessResult struct {
	ExitCode *int
	Stdout   CapturedOutput
	Stderr   CapturedOutput
	Failure  *ExecutionError
}

type ProcessRunner interface {
	Run(context.Context, CommandSpec) ProcessResult
}

type RecordStore interface {
	Save(Record) error
}
