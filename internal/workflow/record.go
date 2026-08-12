package workflow

import (
	"errors"
	"regexp"
	"time"
)

var (
	workflowIDPattern  = regexp.MustCompile(`^wf-[a-f0-9]{32}$`)
	planIDPattern      = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
	operationIDPattern = regexp.MustCompile(`^op-[a-f0-9]{32}$`)
)

func NewRecord(manifest Manifest, workflowID string, createdAt time.Time) (Record, error) {
	if err := ValidateManifest(manifest); err != nil {
		return Record{}, errors.New("new workflow record: manifest is invalid")
	}
	if !workflowIDPattern.MatchString(workflowID) || createdAt.IsZero() ||
		createdAt.Before(manifest.CreatedAt) {
		return Record{}, errors.New("new workflow record: identity or creation time is invalid")
	}
	createdAt = createdAt.UTC()
	value := Record{
		SchemaVersion: RecordSchemaVersion,
		ID:            workflowID,
		ManifestID:    manifest.ID,
		State:         WorkflowPending,
		CreatedAt:     createdAt,
		UpdatedAt:     createdAt,
		Stages: []StageRecord{
			{ID: StageInstallNode, State: StageReady},
			{ID: StageSetDefault, State: StagePending},
			{ID: StageUpdateNodeTools, State: StagePending},
		},
		Transitions: []Transition{{
			StageID: StageInstallNode, From: StagePending, To: StageReady,
			WorkflowState: WorkflowPending, At: createdAt,
			Reason: transitionReason(StageInstallNode, StagePending, StageReady),
		}},
	}
	if err := ValidateRecord(value, manifest); err != nil {
		return Record{}, err
	}
	return value, nil
}

func BindStagePlan(
	record Record,
	manifest Manifest,
	stageID, planID string,
	at time.Time,
) (Record, error) {
	if err := ValidateRecord(record, manifest); err != nil {
		return Record{}, err
	}
	if !planIDPattern.MatchString(planID) || !validNextTime(record, at) {
		return Record{}, errors.New("bind workflow stage Plan: Plan ID or time is invalid")
	}
	result := cloneRecord(record)
	index := stageIndex(stageID)
	if index < 0 || result.Stages[index].State != StageReady ||
		terminalWorkflowState(result.State) {
		return Record{}, errors.New("bind workflow stage Plan: stage is not ready")
	}
	stage := &result.Stages[index]
	stage.PlanID = planID
	stage.PlanSchemaVersion = manifest.Stages[index].PlanSchemaVersion
	stage.State = StagePlanned
	appendTransition(&result, stageID, StageReady, StagePlanned, at)
	if err := ValidateRecord(result, manifest); err != nil {
		return Record{}, err
	}
	return result, nil
}

func StartStage(
	record Record,
	manifest Manifest,
	stageID, operationID string,
	at time.Time,
) (Record, error) {
	if err := ValidateRecord(record, manifest); err != nil {
		return Record{}, err
	}
	if !operationIDPattern.MatchString(operationID) || !validNextTime(record, at) {
		return Record{}, errors.New("start workflow stage: Operation ID or time is invalid")
	}
	result := cloneRecord(record)
	index := stageIndex(stageID)
	if index < 0 || result.Stages[index].State != StagePlanned ||
		terminalWorkflowState(result.State) {
		return Record{}, errors.New("start workflow stage: stage is not planned")
	}
	stage := &result.Stages[index]
	stage.OperationID = operationID
	stage.StartedAt = timePointer(at.UTC())
	stage.State = StageRunning
	if result.StartedAt == nil {
		result.StartedAt = timePointer(at.UTC())
	}
	appendTransition(&result, stageID, StagePlanned, StageRunning, at)
	if err := ValidateRecord(result, manifest); err != nil {
		return Record{}, err
	}
	return result, nil
}

func FinishStage(
	record Record,
	manifest Manifest,
	stageID string,
	terminal StageState,
	checkpointDigest string,
	at time.Time,
) (Record, error) {
	if err := ValidateRecord(record, manifest); err != nil {
		return Record{}, err
	}
	if !terminalStageState(terminal) || !validNextTime(record, at) ||
		(terminal == StageCompleted && !planIDPattern.MatchString(checkpointDigest)) ||
		(checkpointDigest != "" && !planIDPattern.MatchString(checkpointDigest)) {
		return Record{}, errors.New("finish workflow stage: terminal state, checkpoint or time is invalid")
	}
	result := cloneRecord(record)
	index := stageIndex(stageID)
	if index < 0 || result.Stages[index].State != StageRunning ||
		terminalWorkflowState(result.State) {
		return Record{}, errors.New("finish workflow stage: stage is not running")
	}
	stage := &result.Stages[index]
	stage.State = terminal
	stage.CheckpointDigest = checkpointDigest
	stage.FinishedAt = timePointer(at.UTC())
	appendTransition(&result, stageID, StageRunning, terminal, at)
	if terminal == StageCompleted && index+1 < len(result.Stages) {
		next := &result.Stages[index+1]
		next.State = StageReady
		appendTransition(&result, next.ID, StagePending, StageReady, at)
	}
	if terminalWorkflowState(result.State) {
		result.FinishedAt = timePointer(at.UTC())
	}
	if err := ValidateRecord(result, manifest); err != nil {
		return Record{}, err
	}
	return result, nil
}

func ValidateRecord(value Record, manifest Manifest) error {
	if err := ValidateManifest(manifest); err != nil {
		return errors.New("validate workflow record: manifest is invalid")
	}
	if value.SchemaVersion != RecordSchemaVersion ||
		!workflowIDPattern.MatchString(value.ID) ||
		value.ManifestID != manifest.ID ||
		value.CreatedAt.IsZero() || value.CreatedAt.Before(manifest.CreatedAt) ||
		value.UpdatedAt.Before(value.CreatedAt) ||
		len(value.Stages) != len(manifest.Stages) ||
		len(value.Transitions) == 0 {
		return errors.New("validate workflow record: identity, time or stage count is invalid")
	}
	planIDs := make(map[string]bool, len(value.Stages))
	operationIDs := make(map[string]bool, len(value.Stages))
	for index := range value.Stages {
		stage := value.Stages[index]
		if stage.ID != manifest.Stages[index].ID {
			return errors.New("validate workflow record: stage identity or order is invalid")
		}
		if stage.PlanID != "" {
			if planIDs[stage.PlanID] {
				return errors.New("validate workflow record: child Plan ID was reused")
			}
			planIDs[stage.PlanID] = true
		}
		if stage.OperationID != "" {
			if operationIDs[stage.OperationID] {
				return errors.New("validate workflow record: Operation ID was reused")
			}
			operationIDs[stage.OperationID] = true
		}
	}

	simulated := []StageState{StagePending, StagePending, StagePending}
	started := make([]*time.Time, len(simulated))
	finished := make([]*time.Time, len(simulated))
	var workflowStarted *time.Time
	var previous time.Time
	for index, transition := range value.Transitions {
		currentStageIndex := stageIndex(transition.StageID)
		if currentStageIndex < 0 || transition.At.IsZero() ||
			(index == 0 && !transition.At.Equal(value.CreatedAt)) ||
			(index > 0 && transition.At.Before(previous)) ||
			simulated[currentStageIndex] != transition.From ||
			!allowedStageTransition(transition.From, transition.To) ||
			!dependenciesComplete(simulated, currentStageIndex, transition.To) ||
			transition.Reason != transitionReason(transition.StageID, transition.From, transition.To) {
			return errors.New("validate workflow record: transition history is invalid")
		}
		simulated[currentStageIndex] = transition.To
		if transition.To == StageRunning {
			started[currentStageIndex] = timePointer(transition.At)
			if workflowStarted == nil {
				workflowStarted = timePointer(transition.At)
			}
		}
		if terminalStageState(transition.To) {
			finished[currentStageIndex] = timePointer(transition.At)
		}
		expectedWorkflow := stateFromStages(simulated)
		if transition.WorkflowState != expectedWorkflow {
			return errors.New("validate workflow record: transition workflow state is invalid")
		}
		previous = transition.At
	}
	if !value.UpdatedAt.Equal(previous) || value.State != stateFromStages(simulated) ||
		!timesEqual(value.StartedAt, workflowStarted) {
		return errors.New("validate workflow record: final state or timestamps do not match history")
	}
	for index, stage := range value.Stages {
		if stage.State != simulated[index] ||
			!timesEqual(stage.StartedAt, started[index]) ||
			!timesEqual(stage.FinishedAt, finished[index]) ||
			!validStageBinding(stage, manifest.Stages[index]) {
			return errors.New("validate workflow record: stage binding does not match state")
		}
	}
	if terminalWorkflowState(value.State) {
		if value.FinishedAt == nil || !value.FinishedAt.Equal(value.UpdatedAt) {
			return errors.New("validate workflow record: terminal workflow has no matching finish time")
		}
	} else if value.FinishedAt != nil {
		return errors.New("validate workflow record: active workflow has a finish time")
	}
	return nil
}

func appendTransition(record *Record, stageID string, from, to StageState, at time.Time) {
	at = at.UTC()
	states := make([]StageState, len(record.Stages))
	for index, stage := range record.Stages {
		states[index] = stage.State
	}
	record.State = stateFromStages(states)
	record.UpdatedAt = at
	record.Transitions = append(record.Transitions, Transition{
		StageID: stageID, From: from, To: to, WorkflowState: record.State,
		At: at, Reason: transitionReason(stageID, from, to),
	})
}

func validStageBinding(stage StageRecord, definition Stage) bool {
	hasPlan := stage.PlanID != "" || stage.PlanSchemaVersion != ""
	hasOperation := stage.OperationID != ""
	hasCheckpoint := stage.CheckpointDigest != ""
	switch stage.State {
	case StagePending, StageReady:
		return !hasPlan && !hasOperation && !hasCheckpoint &&
			stage.StartedAt == nil && stage.FinishedAt == nil
	case StagePlanned:
		return planIDPattern.MatchString(stage.PlanID) &&
			stage.PlanSchemaVersion == definition.PlanSchemaVersion &&
			!hasOperation && !hasCheckpoint &&
			stage.StartedAt == nil && stage.FinishedAt == nil
	case StageRunning:
		return planIDPattern.MatchString(stage.PlanID) &&
			stage.PlanSchemaVersion == definition.PlanSchemaVersion &&
			operationIDPattern.MatchString(stage.OperationID) &&
			!hasCheckpoint && stage.StartedAt != nil && stage.FinishedAt == nil
	case StageCompleted:
		return planIDPattern.MatchString(stage.PlanID) &&
			stage.PlanSchemaVersion == definition.PlanSchemaVersion &&
			operationIDPattern.MatchString(stage.OperationID) &&
			planIDPattern.MatchString(stage.CheckpointDigest) &&
			stage.StartedAt != nil && stage.FinishedAt != nil
	case StageFailed, StageTimedOut, StageCancelled, StageInterrupted:
		return planIDPattern.MatchString(stage.PlanID) &&
			stage.PlanSchemaVersion == definition.PlanSchemaVersion &&
			operationIDPattern.MatchString(stage.OperationID) &&
			(stage.CheckpointDigest == "" || planIDPattern.MatchString(stage.CheckpointDigest)) &&
			stage.StartedAt != nil && stage.FinishedAt != nil
	default:
		return false
	}
}

func allowedStageTransition(from, to StageState) bool {
	switch from {
	case StagePending:
		return to == StageReady
	case StageReady:
		return to == StagePlanned
	case StagePlanned:
		return to == StageRunning
	case StageRunning:
		return terminalStageState(to)
	default:
		return false
	}
}

func dependenciesComplete(states []StageState, index int, target StageState) bool {
	if target != StageReady {
		for previous := 0; previous < index; previous++ {
			if states[previous] != StageCompleted {
				return false
			}
		}
		return true
	}
	for previous := 0; previous < index; previous++ {
		if states[previous] != StageCompleted {
			return false
		}
	}
	for following := index + 1; following < len(states); following++ {
		if states[following] != StagePending {
			return false
		}
	}
	return true
}

func stateFromStages(states []StageState) WorkflowState {
	for _, state := range states {
		switch state {
		case StageFailed:
			return WorkflowFailed
		case StageTimedOut:
			return WorkflowTimedOut
		case StageCancelled:
			return WorkflowCancelled
		case StageInterrupted:
			return WorkflowInterrupted
		}
	}
	allCompleted := len(states) > 0
	for _, state := range states {
		if state == StageRunning {
			return WorkflowRunning
		}
		allCompleted = allCompleted && state == StageCompleted
	}
	if allCompleted {
		return WorkflowCompleted
	}
	return WorkflowPending
}

func terminalStageState(state StageState) bool {
	switch state {
	case StageCompleted, StageFailed, StageTimedOut, StageCancelled, StageInterrupted:
		return true
	default:
		return false
	}
}

func terminalWorkflowState(state WorkflowState) bool {
	switch state {
	case WorkflowCompleted, WorkflowFailed, WorkflowTimedOut, WorkflowCancelled, WorkflowInterrupted:
		return true
	default:
		return false
	}
}

func stageIndex(stageID string) int {
	switch stageID {
	case StageInstallNode:
		return 0
	case StageSetDefault:
		return 1
	case StageUpdateNodeTools:
		return 2
	default:
		return -1
	}
}

func transitionReason(stageID string, from, to StageState) string {
	return stageID + " stage transitioned from " + string(from) + " to " + string(to)
}

func validNextTime(record Record, at time.Time) bool {
	return !at.IsZero() && !at.Before(record.UpdatedAt)
}

func timesEqual(left, right *time.Time) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Equal(*right)
}

func cloneRecord(value Record) Record {
	result := value
	result.StartedAt = cloneTime(value.StartedAt)
	result.FinishedAt = cloneTime(value.FinishedAt)
	result.Stages = make([]StageRecord, len(value.Stages))
	for index, stage := range value.Stages {
		result.Stages[index] = stage
		result.Stages[index].StartedAt = cloneTime(stage.StartedAt)
		result.Stages[index].FinishedAt = cloneTime(stage.FinishedAt)
	}
	result.Transitions = append([]Transition{}, value.Transitions...)
	return result
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	return timePointer(*value)
}
