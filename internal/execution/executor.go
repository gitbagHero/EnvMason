package execution

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

type Executor struct {
	Registry       Registry
	Runner         ProcessRunner
	Store          RecordStore
	Now            func() time.Time
	NewOperationID func() (string, error)
}

type preparedAction struct {
	action     plan.Action
	definition Definition
	spec       CommandSpec
}

func (executor Executor) Execute(ctx context.Context, request Request) (Record, error) {
	now := executor.now()
	if err := validateRequest(request, now); err != nil {
		return Record{}, err
	}
	if executor.Runner == nil || executor.Store == nil {
		return Record{}, executionError(CodePlanInvalid, "executor dependencies are incomplete")
	}
	actions, err := orderedActions(request.Plan.Actions)
	if err != nil {
		return Record{}, executionError(CodePlanInvalid, "plan action order is invalid")
	}
	prepared := make([]preparedAction, 0, len(actions))
	for _, action := range actions {
		definition, err := executor.Registry.Resolve(action)
		if err != nil {
			return Record{}, err
		}
		spec, err := definition.Build(action)
		if err != nil || !validCommandSpec(spec) {
			return Record{}, executionError(CodePlanInvalid, "registered action produced an invalid process specification")
		}
		prepared = append(prepared, preparedAction{action: action, definition: definition, spec: spec})
	}

	id, err := executor.operationID()
	if err != nil {
		return Record{}, executionError(CodeLogWriteFailed, "operation identity could not be created")
	}
	record := newRecord(id, request, actions, now)
	if err := executor.Store.Save(record); err != nil {
		return Record{}, executionError(CodeLogWriteFailed, "initial operation record could not be persisted")
	}

	for index, item := range prepared {
		if ctx.Err() != nil {
			failure := executionError(CodeCancelled, "execution was cancelled before the next action")
			return executor.finishFailure(record, index, StateCancelled, failure)
		}
		redactor := NewRedactor(append(append([]string{}, request.SensitiveValues...), item.spec.SensitiveValues...)...)
		startedAt := executor.now()
		record.State = StateRunning
		record.UpdatedAt = startedAt
		if record.StartedAt == nil {
			record.StartedAt = timePointer(startedAt)
		}
		step := &record.Steps[index]
		step.State = StateRunning
		step.StartedAt = timePointer(startedAt)
		step.Invocation = &Invocation{Executable: redactor.String(item.spec.Executable), Args: redactor.Strings(item.spec.Args)}
		step.Precondition = CheckResult{State: CheckPending}
		step.Verification = CheckResult{State: CheckNotRun}
		record.Transitions = append(record.Transitions, Transition{State: StateRunning, At: startedAt, ActionID: item.action.ID, Reason: "registered action started"})
		if err := executor.Store.Save(record); err != nil {
			return executor.logFailure(record, index)
		}

		if item.definition.Preflight != nil {
			if err := item.definition.Preflight(ctx, item.action); err != nil {
				step.Precondition = CheckResult{State: CheckFailed, Message: "registered precondition failed"}
				failure := executionError(CodePreconditionFailed, "registered action precondition failed")
				return executor.finishFailure(record, index, StateFailed, failure)
			}
		}
		step.Precondition = CheckResult{State: CheckPassed, Message: "registered precondition passed"}
		if item.definition.Capture != nil {
			before, err := item.definition.Capture(ctx, item.action)
			if err != nil {
				step.Precondition = CheckResult{State: CheckFailed, Message: "registered state capture failed"}
				failure := executionError(CodePreconditionFailed, "registered action state could not be captured")
				return executor.finishFailure(record, index, StateFailed, failure)
			}
			step.Before = &before
		}
		record.UpdatedAt = executor.now()
		if err := executor.Store.Save(record); err != nil {
			return executor.logFailure(record, index)
		}

		satisfied := false
		if item.definition.Satisfied != nil {
			var err error
			satisfied, err = item.definition.Satisfied(ctx, item.action)
			if err != nil {
				step.Precondition = CheckResult{State: CheckFailed, Message: "registered idempotency check failed"}
				failure := executionError(CodePreconditionFailed, "registered action idempotency check failed")
				return executor.finishFailure(record, index, StateFailed, failure)
			}
		}
		result := ProcessResult{}
		if satisfied {
			code := 0
			result.ExitCode = &code
			step.Skipped = true
		} else {
			result = executor.Runner.Run(ctx, item.spec)
		}
		result.Stdout = sanitizeOutput(result.Stdout, redactor)
		result.Stderr = sanitizeOutput(result.Stderr, redactor)
		step.Stdout = result.Stdout
		step.Stderr = result.Stderr
		step.ExitCode = result.ExitCode
		if result.Failure != nil {
			executor.captureAfter(ctx, item, step)
			state := failureState(result.Failure.Code)
			return executor.finishFailure(record, index, state, result.Failure)
		}

		verifyingAt := executor.now()
		record.State = StateVerifying
		record.UpdatedAt = verifyingAt
		step.State = StateVerifying
		step.Verification = CheckResult{State: CheckPending}
		reason := "process succeeded; registered verification started"
		if satisfied {
			reason = "target state already satisfied; process skipped and registered verification started"
		}
		record.Transitions = append(record.Transitions, Transition{State: StateVerifying, At: verifyingAt, ActionID: item.action.ID, Reason: reason})
		if err := executor.captureAfter(ctx, item, step); err != nil {
			step.Verification = CheckResult{State: CheckFailed, Message: "registered state capture failed"}
			failure := executionError(CodeVerificationFailed, "registered action state could not be captured after execution")
			return executor.finishFailure(record, index, StateFailed, failure)
		}
		if err := executor.Store.Save(record); err != nil {
			return executor.logFailure(record, index)
		}
		if err := item.definition.Verify(ctx, item.action, result); err != nil {
			step.Verification = CheckResult{State: CheckFailed, Message: "registered verification failed"}
			failure := executionError(CodeVerificationFailed, "registered action verification failed")
			return executor.finishFailure(record, index, StateFailed, failure)
		}

		finishedAt := executor.now()
		step.State = StateCompleted
		step.FinishedAt = timePointer(finishedAt)
		step.Verification = CheckResult{State: CheckPassed, Message: "registered verification passed"}
		record.UpdatedAt = finishedAt
		if index < len(prepared)-1 {
			record.State = StateRunning
			record.Transitions = append(record.Transitions, Transition{State: StateRunning, At: finishedAt, ActionID: item.action.ID, Reason: "action completed; next dependency-ready action pending"})
			if err := executor.Store.Save(record); err != nil {
				return executor.logFailure(record, index)
			}
			continue
		}
		record.State = StateCompleted
		record.FinishedAt = timePointer(finishedAt)
		record.Transitions = append(record.Transitions, Transition{State: StateCompleted, At: finishedAt, ActionID: item.action.ID, Reason: "all actions and verifications completed"})
		if err := executor.Store.Save(record); err != nil {
			return executor.logFailure(record, index)
		}
	}
	return record, nil
}

func (executor Executor) captureAfter(ctx context.Context, item preparedAction, step *StepRecord) error {
	if item.definition.Capture == nil {
		return nil
	}
	after, err := item.definition.Capture(ctx, item.action)
	if err != nil {
		return err
	}
	step.After = &after
	if step.Before != nil {
		step.Diff = DiffSnapshots(*step.Before, after)
	}
	return nil
}

func validateRequest(request Request, now time.Time) error {
	if err := plan.Validate(request.Plan); err != nil {
		return executionError(CodePlanInvalid, "plan failed immutable content validation")
	}
	if (request.Plan.SchemaVersion != plan.ExecutableSchemaVersion && request.Plan.SchemaVersion != plan.HighRiskExecutableSchemaVersion) || !request.Plan.Executable {
		return executionError(CodePlanInvalid, "only executable Plan 0.2.0 or 0.3.0 is accepted")
	}
	if !now.Before(request.Plan.ExpiresAt) {
		return executionError(CodePlanExpired, "plan has expired")
	}
	confirmation := request.Confirmation
	if confirmation.Scope != "plan" || confirmation.ConfirmedPlanID != request.Plan.ID || confirmation.ConfirmedAt.Before(request.Plan.CreatedAt) || confirmation.ConfirmedAt.After(now) {
		return executionError(CodeConfirmationRequired, "current user confirmation is not bound to this Plan ID")
	}
	return nil
}

func validCommandSpec(spec CommandSpec) bool {
	if !filepath.IsAbs(spec.Executable) || spec.Timeout <= 0 || spec.Timeout > MaximumTimeout || (spec.Directory != "" && !filepath.IsAbs(spec.Directory)) {
		return false
	}
	total := len(spec.Executable) + len(spec.Directory)
	for _, argument := range spec.Args {
		if strings.IndexByte(argument, 0) >= 0 {
			return false
		}
		total += len(argument)
	}
	return total <= DefaultOutputLimit
}

func (executor Executor) finishFailure(record Record, stepIndex int, state State, failure *ExecutionError) (Record, error) {
	finishedAt := executor.now()
	record.State = state
	record.UpdatedAt = finishedAt
	record.FinishedAt = timePointer(finishedAt)
	step := &record.Steps[stepIndex]
	step.State = state
	step.FinishedAt = timePointer(finishedAt)
	step.Error = &ErrorDetail{Code: failure.Code, Message: failure.Message}
	if step.Verification.State == CheckPending {
		step.Verification = CheckResult{State: CheckFailed, Message: "verification did not complete"}
	}
	record.Transitions = append(record.Transitions, Transition{State: state, At: finishedAt, ActionID: step.ActionID, Reason: failure.Message})
	if err := executor.Store.Save(record); err != nil {
		return executor.logFailure(record, stepIndex)
	}
	return record, failure
}

func (executor Executor) logFailure(record Record, stepIndex int) (Record, error) {
	failedAt := executor.now()
	if len(record.Transitions) > 1 && terminalState(record.Transitions[len(record.Transitions)-1].State) {
		record.Transitions = record.Transitions[:len(record.Transitions)-1]
	}
	record.State = StateFailed
	record.UpdatedAt = failedAt
	record.FinishedAt = timePointer(failedAt)
	if stepIndex >= 0 && stepIndex < len(record.Steps) {
		step := &record.Steps[stepIndex]
		step.State = StateFailed
		step.FinishedAt = timePointer(failedAt)
		step.Error = &ErrorDetail{Code: CodeLogWriteFailed, Message: "operation record could not be persisted"}
	}
	record.Transitions = append(record.Transitions, Transition{State: StateFailed, At: failedAt, Reason: "operation record could not be persisted"})
	return record, executionError(CodeLogWriteFailed, "operation record could not be persisted")
}

func newRecord(id string, request Request, actions []plan.Action, now time.Time) Record {
	steps := make([]StepRecord, 0, len(actions))
	for _, action := range actions {
		steps = append(steps, StepRecord{
			ActionID: action.ID, ToolID: action.ToolID, Operation: action.Operation, Adapter: action.Adapter, Risk: action.Risk,
			State: StatePending, Precondition: CheckResult{State: CheckPending}, Verification: CheckResult{State: CheckNotRun},
			Stdout: CapturedOutput{}, Stderr: CapturedOutput{},
		})
	}
	return Record{
		SchemaVersion: RecordSchemaVersion, ID: id, PlanID: request.Plan.ID, PlanSchemaVersion: request.Plan.SchemaVersion,
		State: StatePending, CreatedAt: now, UpdatedAt: now, Confirmation: request.Confirmation, Steps: steps,
		Transitions: []Transition{{State: StatePending, At: now, Reason: "confirmed immutable Plan accepted"}},
	}
}

func orderedActions(actions []plan.Action) ([]plan.Action, error) {
	byID := make(map[string]plan.Action, len(actions))
	for _, action := range actions {
		byID[action.ID] = action
	}
	state := make(map[string]uint8, len(actions))
	ordered := make([]plan.Action, 0, len(actions))
	var visit func(string) error
	visit = func(id string) error {
		if state[id] == 1 {
			return errors.New("dependency cycle")
		}
		if state[id] == 2 {
			return nil
		}
		action, ok := byID[id]
		if !ok {
			return errors.New("unknown dependency")
		}
		state[id] = 1
		for _, dependency := range action.Dependencies {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[id] = 2
		ordered = append(ordered, action)
		return nil
	}
	for _, action := range actions {
		if err := visit(action.ID); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

func sanitizeOutput(output CapturedOutput, redactor Redactor) CapturedOutput {
	text, truncated := boundUTF8(output.Text, DefaultOutputLimit)
	text = redactor.String(text)
	text, redactedTruncated := boundUTF8(text, DefaultOutputLimit)
	return CapturedOutput{Text: text, Truncated: output.Truncated || truncated || redactedTruncated}
}

func boundUTF8(value string, limit int) (string, bool) {
	if len(value) <= limit {
		return value, false
	}
	value = value[:limit]
	for !utf8.ValidString(value) && len(value) > 0 {
		value = value[:len(value)-1]
	}
	return value, true
}

func failureState(code Code) State {
	switch code {
	case CodeTimeout:
		return StateTimedOut
	case CodeCancelled:
		return StateCancelled
	default:
		return StateFailed
	}
}

func terminalState(state State) bool {
	return state == StateCompleted || state == StateFailed || state == StateTimedOut || state == StateCancelled || state == StateInterrupted
}

func (executor Executor) now() time.Time {
	if executor.Now != nil {
		return executor.Now().UTC()
	}
	return time.Now().UTC()
}

func (executor Executor) operationID() (string, error) {
	if executor.NewOperationID != nil {
		return executor.NewOperationID()
	}
	var random [16]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	return "op-" + hex.EncodeToString(random[:]), nil
}

func timePointer(value time.Time) *time.Time { return &value }

func RecoverInterrupted(record Record, observedAt time.Time) (Record, error) {
	if record.State != StateRunning && record.State != StateVerifying {
		return Record{}, errors.New("recover operation record: only active records can be interrupted")
	}
	observedAt = observedAt.UTC()
	if observedAt.Before(record.UpdatedAt) {
		return Record{}, errors.New("recover operation record: observation predates record")
	}
	for index := range record.Steps {
		if record.Steps[index].State == StateRunning || record.Steps[index].State == StateVerifying {
			record.Steps[index].State = StateInterrupted
			record.Steps[index].FinishedAt = timePointer(observedAt)
			record.Steps[index].Error = &ErrorDetail{Code: CodeInterrupted, Message: "previous process ended without a terminal audit state"}
			if record.Steps[index].Verification.State == CheckPending {
				record.Steps[index].Verification = CheckResult{State: CheckFailed, Message: "verification was interrupted"}
			}
		}
	}
	record.State = StateInterrupted
	record.UpdatedAt = observedAt
	record.FinishedAt = timePointer(observedAt)
	record.Transitions = append(record.Transitions, Transition{State: StateInterrupted, At: observedAt, Reason: "active record recovered after process interruption"})
	if err := ValidateRecord(record); err != nil {
		return Record{}, err
	}
	return record, nil
}
