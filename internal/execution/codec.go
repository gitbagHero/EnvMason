package execution

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	operationschema "github.com/gitbagHero/EnvMason/schemas/operation"
)

var operationIDPattern = regexp.MustCompile(`^op-[a-f0-9]{32}$`)
var planIDPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

var compiledRecordSchema = struct {
	sync.Mutex
	value *jsonschema.Schema
}{}

func MarshalRecord(value Record) ([]byte, error) {
	if err := ValidateRecord(value); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode operation record: %w", err)
	}
	data = append(data, '\n')
	if err := ValidateRecordJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func DecodeRecord(data []byte) (Record, error) {
	if err := ValidateRecordJSON(data); err != nil {
		return Record{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value Record
	if err := decoder.Decode(&value); err != nil {
		return Record{}, fmt.Errorf("decode operation record: %w", err)
	}
	if _, err := decoder.Token(); err == nil || err != io.EOF {
		return Record{}, errors.New("decode operation record: trailing JSON value")
	}
	if err := ValidateRecord(value); err != nil {
		return Record{}, err
	}
	return value, nil
}

func ValidateRecordJSON(data []byte) error {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("parse operation record JSON: %w", err)
	}
	if envelope.SchemaVersion != RecordSchemaVersion {
		return fmt.Errorf("validate operation record JSON: unsupported schema_version %q", envelope.SchemaVersion)
	}
	schema, err := recordSchema()
	if err != nil {
		return fmt.Errorf("compile operation record schema: %w", err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse operation record JSON: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate operation record JSON: %w", err)
	}
	return nil
}

func ValidateRecord(value Record) error {
	if value.SchemaVersion != RecordSchemaVersion || !operationIDPattern.MatchString(value.ID) || !planIDPattern.MatchString(value.PlanID) || value.PlanSchemaVersion == "" {
		return errors.New("validate operation record: identity is incomplete")
	}
	if value.CreatedAt.IsZero() || value.UpdatedAt.Before(value.CreatedAt) || len(value.Steps) == 0 || len(value.Transitions) == 0 {
		return errors.New("validate operation record: timestamps, steps and transitions are required")
	}
	if value.Confirmation.Scope != "plan" || value.Confirmation.ConfirmedPlanID != value.PlanID || value.Confirmation.ConfirmedAt.IsZero() || value.Confirmation.ConfirmedAt.After(value.CreatedAt) {
		return errors.New("validate operation record: confirmation does not match Plan ID")
	}
	if value.Transitions[0].State != StatePending || value.Transitions[len(value.Transitions)-1].State != value.State {
		return errors.New("validate operation record: transition history does not match state")
	}
	knownActions := make(map[string]bool, len(value.Steps))
	for _, step := range value.Steps {
		if step.ActionID == "" || knownActions[step.ActionID] || !validState(step.State) {
			return errors.New("validate operation record: invalid or duplicated step")
		}
		knownActions[step.ActionID] = true
		if step.StartedAt != nil && step.FinishedAt != nil && step.FinishedAt.Before(*step.StartedAt) {
			return errors.New("validate operation record: step finish predates start")
		}
		if step.State == StateCompleted && (step.FinishedAt == nil || step.Verification.State != CheckPassed || step.Error != nil) {
			return errors.New("validate operation record: completed step is not verified")
		}
		if terminalState(step.State) && step.FinishedAt == nil {
			return errors.New("validate operation record: terminal step has no finish time")
		}
	}
	if !validState(value.State) || (value.StartedAt != nil && value.FinishedAt != nil && value.FinishedAt.Before(*value.StartedAt)) {
		return errors.New("validate operation record: invalid state or duration")
	}
	if terminalState(value.State) && value.FinishedAt == nil {
		return errors.New("validate operation record: terminal record has no finish time")
	}
	for index, transition := range value.Transitions {
		if transition.At.IsZero() || transition.Reason == "" || (index > 0 && transition.At.Before(value.Transitions[index-1].At)) {
			return errors.New("validate operation record: invalid transition")
		}
		if transition.ActionID != "" && !knownActions[transition.ActionID] {
			return errors.New("validate operation record: transition references unknown action")
		}
		if index > 0 && !validTransition(value.Transitions[index-1].State, transition.State) {
			return errors.New("validate operation record: illegal state transition")
		}
	}
	if value.State == StateCompleted {
		if value.FinishedAt == nil {
			return errors.New("validate operation record: completed record has no finish time")
		}
		for _, step := range value.Steps {
			if step.State != StateCompleted || step.Verification.State != CheckPassed || step.FinishedAt == nil {
				return errors.New("validate operation record: completed record contains an incomplete step")
			}
		}
	}
	return nil
}

func validState(state State) bool {
	switch state {
	case StatePending, StateRunning, StateVerifying, StateCompleted, StateFailed, StateTimedOut, StateCancelled, StateInterrupted:
		return true
	default:
		return false
	}
}

func validTransition(from, to State) bool {
	switch from {
	case StatePending:
		return to == StateRunning || to == StateFailed || to == StateCancelled
	case StateRunning:
		return to == StateRunning || to == StateVerifying || to == StateFailed || to == StateTimedOut || to == StateCancelled || to == StateInterrupted
	case StateVerifying:
		return to == StateRunning || to == StateCompleted || to == StateFailed || to == StateCancelled || to == StateInterrupted
	default:
		return false
	}
}

func recordSchema() (*jsonschema.Schema, error) {
	compiledRecordSchema.Lock()
	defer compiledRecordSchema.Unlock()
	if compiledRecordSchema.value != nil {
		return compiledRecordSchema.value, nil
	}
	data, id, ok := operationschema.ByVersion(RecordSchemaVersion)
	if !ok {
		return nil, fmt.Errorf("unsupported schema_version %q", RecordSchemaVersion)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(id, document); err != nil {
		return nil, err
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		return nil, err
	}
	compiledRecordSchema.value = schema
	return schema, nil
}
