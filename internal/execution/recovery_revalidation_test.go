package execution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestRevalidateRecoveryClassifiesCurrentDriftedAndUnavailableInOrder(t *testing.T) {
	t.Parallel()
	source, _, _, _ := checkpointSource(t, "update-pnpm", "", "", "", false)
	setStepChange(t, &source.Steps[0], "secret-npm-state", "before", "after")
	setStepChange(t, &source.Steps[1], "secret-corepack-state", "before", "after")
	setStepChange(t, &source.Steps[2], "secret-pnpm-state", "before", "after")
	order := []string{}
	registry := recoveryRevalidationRegistry(t, source, map[string]func(plan.Action, Snapshot) (Snapshot, error){
		"update-npm": func(action plan.Action, recorded Snapshot) (Snapshot, error) {
			order = append(order, action.ID)
			return NewSnapshot(recorded.Facts)
		},
		"update-corepack": func(action plan.Action, _ Snapshot) (Snapshot, error) {
			order = append(order, action.ID)
			return Snapshot{}, errors.New("secret /private/drift")
		},
	})

	result, err := RevalidateRecovery(t.Context(), source, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 3 ||
		result.Candidates[0].ActionID != "update-npm" ||
		result.Candidates[0].CurrentState != RecoveryCheckpointCurrent ||
		result.Candidates[1].ActionID != "update-corepack" ||
		result.Candidates[1].CurrentState != RecoveryCheckpointDrifted ||
		result.Candidates[2].ActionID != "update-pnpm" ||
		result.Candidates[2].CurrentState != RecoveryCheckpointVerifierUnavailable {
		t.Fatalf("revalidation = %#v", result)
	}
	if strings.Join(order, ",") != "update-npm,update-corepack" {
		t.Fatalf("callback order = %v", order)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"secret-npm-state", "secret-corepack-state", "secret-pnpm-state", "/private/drift"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("revalidation leaked %q: %s", secret, data)
		}
	}
}

func TestRevalidateRecoveryDoesNotProbeUncertainCandidate(t *testing.T) {
	t.Parallel()
	source, _, _, _ := checkpointSource(t, "update-corepack", "", "", "", false)
	source.Steps[0].Skipped = true
	source.Steps[0].Diff = nil
	source.Steps[1].After = nil
	source.Steps[1].Diff = nil
	source.Steps[1].Invocation = &Invocation{Executable: "/secret/write", Args: []string{"secret"}}
	source.Steps[1].Error = &ErrorDetail{Code: CodeExitNonZero, Message: "secret failure"}
	calls := 0
	registry := recoveryRevalidationRegistry(t, source, map[string]func(plan.Action, Snapshot) (Snapshot, error){
		"update-corepack": func(_ plan.Action, _ Snapshot) (Snapshot, error) {
			calls++
			return Snapshot{}, nil
		},
	})

	result, err := RevalidateRecovery(t.Context(), source, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 ||
		result.Candidates[0].ActionID != "update-corepack" ||
		result.Candidates[0].CurrentState != RecoveryCheckpointUncertain ||
		calls != 0 {
		t.Fatalf("uncertain revalidation = %#v, calls = %d", result, calls)
	}
}

func TestRevalidateRecoveryRejectsInvalidCallbackEvidenceWithoutOtherCallbacks(t *testing.T) {
	t.Parallel()
	source, _, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
	setStepChange(t, &source.Steps[0], "secret-state", "before", "after")
	otherCalls := 0
	registry := recoveryRevalidationRegistryWithCounters(
		t,
		source,
		func(_ plan.Action, recorded Snapshot) (Snapshot, error) {
			recorded.Facts["mutated_callback_copy"] = "secret mutation"
			return Snapshot{Digest: recorded.Digest, Facts: map[string]string{"different": "evidence"}}, nil
		},
		&otherCalls,
	)
	before, err := MarshalRecord(source)
	if err != nil {
		t.Fatal(err)
	}

	result, err := RevalidateRecovery(t.Context(), source, registry)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 1 ||
		result.Candidates[0].CurrentState != RecoveryCheckpointDrifted {
		t.Fatalf("invalid callback evidence = %#v", result)
	}
	if otherCalls != 0 {
		t.Fatalf("non-revalidation callbacks called %d times", otherCalls)
	}
	after, err := MarshalRecord(source)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("revalidation or callback mutated the source record")
	}
	if _, present := source.Steps[0].After.Facts["mutated_callback_copy"]; present {
		t.Fatal("callback mutation escaped its Snapshot copy")
	}
}

func TestRevalidateRecoveryReturnsContextCancellationWithoutCallbacks(t *testing.T) {
	t.Parallel()
	source, _, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
	setStepChange(t, &source.Steps[0], "state", "before", "after")
	calls := 0
	registry := recoveryRevalidationRegistry(t, source, map[string]func(plan.Action, Snapshot) (Snapshot, error){
		"update-npm": func(_ plan.Action, _ Snapshot) (Snapshot, error) {
			calls++
			return Snapshot{}, nil
		},
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	result, err := RevalidateRecovery(ctx, source, registry)
	if !errors.Is(err, context.Canceled) || calls != 0 ||
		result.SourceOperationID != "" || len(result.Candidates) != 0 {
		t.Fatalf("cancelled revalidation = %#v, %v, calls = %d", result, err, calls)
	}
}

func TestRevalidateRecoveryDiscardsCallbackResultWhenContextIsCancelled(t *testing.T) {
	t.Parallel()
	source, _, _, _ := checkpointSource(t, "update-npm", "", "", "", false)
	setStepChange(t, &source.Steps[0], "state", "before", "after")
	ctx, cancel := context.WithCancel(t.Context())
	registry := recoveryRevalidationRegistry(t, source, map[string]func(plan.Action, Snapshot) (Snapshot, error){
		"update-npm": func(_ plan.Action, recorded Snapshot) (Snapshot, error) {
			cancel()
			return NewSnapshot(recorded.Facts)
		},
	})

	result, err := RevalidateRecovery(ctx, source, registry)
	if !errors.Is(err, context.Canceled) ||
		result.SourceOperationID != "" || len(result.Candidates) != 0 {
		t.Fatalf("callback-cancelled revalidation = %#v, %v", result, err)
	}
}

func recoveryRevalidationRegistry(
	t *testing.T,
	source Record,
	revalidators map[string]func(plan.Action, Snapshot) (Snapshot, error),
) Registry {
	t.Helper()
	definitions := make([]Definition, 0, len(revalidators))
	for _, action := range source.ConfirmedPlan.Actions {
		revalidate, ok := revalidators[action.ID]
		if !ok {
			continue
		}
		action := action
		definitions = append(definitions, Definition{
			Key:         ActionKey{ToolID: action.ToolID, Operation: action.Operation, Adapter: action.Adapter},
			MinimumRisk: action.Risk,
			Build: func(plan.Action) (CommandSpec, error) {
				return CommandSpec{}, errors.New("Build must not be called")
			},
			Verify: func(context.Context, plan.Action, ProcessResult) error {
				return errors.New("Verify must not be called")
			},
			RevalidateCheckpoint: func(_ context.Context, candidate plan.Action, recorded Snapshot) (Snapshot, error) {
				return revalidate(candidate, recorded)
			},
		})
	}
	registry, err := NewRegistry(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func recoveryRevalidationRegistryWithCounters(
	t *testing.T,
	source Record,
	revalidate func(plan.Action, Snapshot) (Snapshot, error),
	otherCalls *int,
) Registry {
	t.Helper()
	action := source.ConfirmedPlan.Actions[0]
	definition := Definition{
		Key:         ActionKey{ToolID: action.ToolID, Operation: action.Operation, Adapter: action.Adapter},
		MinimumRisk: action.Risk,
		Build: func(plan.Action) (CommandSpec, error) {
			*otherCalls++
			return CommandSpec{}, nil
		},
		Preflight: func(context.Context, plan.Action) error {
			*otherCalls++
			return nil
		},
		Capture: func(context.Context, plan.Action) (Snapshot, error) {
			*otherCalls++
			return Snapshot{}, nil
		},
		Satisfied: func(context.Context, plan.Action) (bool, error) {
			*otherCalls++
			return false, nil
		},
		Verify: func(context.Context, plan.Action, ProcessResult) error {
			*otherCalls++
			return nil
		},
		RevalidateCheckpoint: func(_ context.Context, candidate plan.Action, recorded Snapshot) (Snapshot, error) {
			if len(candidate.Preconditions) > 0 {
				candidate.Preconditions[0].Expected = "secret callback mutation"
			}
			return revalidate(candidate, recorded)
		},
	}
	registry, err := NewRegistry(definition)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
