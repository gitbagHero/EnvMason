package baseapply

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestFinalizeProducesDeterministicFinalLockAndRedactedDiff(t *testing.T) {
	requireMacOSExecutorFixture(t)
	input := validFinalizeInput(t, true)
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}

	first, err := Finalize(input)
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	second, err := Finalize(input)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("deterministic Finalize() = %#v, %v", second, err)
	}
	after, err := json.Marshal(input)
	if err != nil || string(before) != string(after) {
		t.Fatal("Finalize mutated its explicit evidence")
	}

	if first.OperationID != input.Record.ID || first.PlanID != input.Prepared.Plan.ID ||
		first.ReviewID != input.Prepared.Review.ID || first.Diff.PreviousLockID != input.Prepared.Lock.ID ||
		first.Diff.FinalLockID != first.FinalLock.ID || first.FinalLock.ID == input.Prepared.Lock.ID ||
		!first.FinalLock.GeneratedAt.Equal(input.FinalizedAt) ||
		first.FinalLock.Profile != input.Prepared.Lock.Profile ||
		first.FinalLock.Target != input.Prepared.Lock.Target ||
		!reflect.DeepEqual(first.FinalLock.Sources, input.Prepared.Lock.Sources) ||
		first.FinalLock.Summary != (lockfile.Summary{Satisfied: 2}) {
		t.Fatalf("outcome identity = %#v", first)
	}
	wantChanges := []LockChange{
		{
			ItemID: "base.cmake", Capability: "base.cmake", ToolID: "homebrew.formula.cmake",
			Manager: "homebrew", Version: "4.0.3", BeforeState: lockfile.StateInstallRequired,
			AfterState: lockfile.StateSatisfied,
		},
		{
			ItemID: "base.git", Capability: "base.git", ToolID: "homebrew.formula.git",
			Manager: "homebrew", Version: "2.51.0", BeforeState: lockfile.StateInstallRequired,
			AfterState: lockfile.StateSatisfied,
		},
	}
	if !reflect.DeepEqual(first.Diff.Changes, wantChanges) {
		t.Fatalf("diff changes = %#v", first.Diff.Changes)
	}
	for _, item := range first.FinalLock.Items {
		if item.State != lockfile.StateSatisfied || len(item.Observed) != 1 ||
			item.Observed[0].Manager != "homebrew" || item.Observed[0].Version != item.Implementation.Version ||
			item.Reason != lockfile.ReasonCompatibleInstallation {
			t.Fatalf("final item = %#v", item)
		}
	}

	data, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"/Users/envmason", "/private/tmp/envmason", "/opt/homebrew"} {
		if strings.Contains(string(data), private) {
			t.Fatalf("Outcome leaked private execution evidence %q", private)
		}
	}
	if _, err := plan.BuildBaseInstall(plan.BaseInstallInput{
		Lock: first.FinalLock, Inventory: input.Inventory, CreatedAt: input.FinalizedAt.Add(time.Second),
	}); err == nil {
		t.Fatal("final Lock incorrectly produced another Base installation Plan")
	}
}

func TestFinalizeChangesOnlyReviewedInstallRequiredItems(t *testing.T) {
	requireMacOSExecutorFixture(t)
	input := validFinalizeInput(t, false)
	outcome, err := Finalize(input)
	if err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}
	if len(outcome.Diff.Changes) != 1 || outcome.Diff.Changes[0].ItemID != "base.git" ||
		outcome.FinalLock.Summary != (lockfile.Summary{Satisfied: 2}) {
		t.Fatalf("single-action Outcome = %#v", outcome)
	}
	for _, item := range outcome.FinalLock.Items {
		if item.ID == "base.cmake" && !reflect.DeepEqual(item, input.Prepared.Lock.Items[0]) {
			t.Fatalf("pre-satisfied item changed = %#v", item)
		}
	}
}

func TestFinalizeRejectsIncompleteOrDriftedEvidence(t *testing.T) {
	requireMacOSExecutorFixture(t)
	tests := []struct {
		name   string
		mutate func(*FinalizeInput)
	}{
		{"zero finalization time", func(value *FinalizeInput) { value.FinalizedAt = time.Time{} }},
		{"invalid Prepared binding", func(value *FinalizeInput) { value.Prepared.Review.ID = applyDigest('f') }},
		{"failed Operation", func(value *FinalizeInput) {
			value.Record.State = execution.StateFailed
			value.Record.Transitions[len(value.Record.Transitions)-1].State = execution.StateFailed
		}},
		{"old Operation schema", func(value *FinalizeInput) {
			value.Record.SchemaVersion = execution.PreviousRecordSchemaVersion
		}},
		{"finish after finalization", func(value *FinalizeInput) {
			finishedAt := value.FinalizedAt.Add(time.Second)
			value.Record.FinishedAt = &finishedAt
			value.Record.UpdatedAt = finishedAt
			value.Record.Transitions[len(value.Record.Transitions)-1].At = finishedAt
		}},
		{"missing before snapshot", func(value *FinalizeInput) {
			value.Record.Steps[0].Before = nil
			value.Record.Steps[0].Diff = nil
		}},
		{"forged initial snapshot", func(value *FinalizeInput) {
			step := &value.Record.Steps[0]
			facts := make(map[string]string, len(step.Before.Facts))
			for key, fact := range step.Before.Facts {
				facts[key] = fact
			}
			facts["formula.gettext"] = "0.0.0"
			before, err := execution.NewSnapshot(facts)
			if err != nil {
				t.Fatal(err)
			}
			step.Before = &before
			step.Diff = execution.DiffSnapshots(before, *step.After)
		}},
		{"forged final snapshot", func(value *FinalizeInput) {
			step := &value.Record.Steps[0]
			facts := make(map[string]string, len(step.After.Facts))
			for key, fact := range step.After.Facts {
				facts[key] = fact
			}
			for key := range facts {
				if strings.HasPrefix(key, "formula.") {
					facts[key] = "0.0.0"
					break
				}
			}
			after, err := execution.NewSnapshot(facts)
			if err != nil {
				t.Fatal(err)
			}
			step.After = &after
			step.Diff = execution.DiffSnapshots(*step.Before, after)
		}},
		{"Inventory time mismatch", func(value *FinalizeInput) {
			value.Inventory.GeneratedAt = value.FinalizedAt.Add(-time.Second)
		}},
		{"target drift", func(value *FinalizeInput) { value.Inventory.System.OSVersion = "14.0" }},
		{"active Homebrew identity drift", func(value *FinalizeInput) {
			value.Inventory.Tools[0].Installations[0].Version = "6.1.0"
		}},
		{"active Homebrew manager drift", func(value *FinalizeInput) {
			value.Inventory.Tools[0].Installations[0].Manager = "unknown"
		}},
		{"missing reviewed formula", func(value *FinalizeInput) {
			for index, tool := range value.Inventory.Tools {
				if tool.ID == "homebrew.formula.git" {
					value.Inventory.Tools = append(value.Inventory.Tools[:index], value.Inventory.Tools[index+1:]...)
					return
				}
			}
		}},
		{"duplicate reviewed formula", func(value *FinalizeInput) {
			for index := range value.Inventory.Tools {
				if value.Inventory.Tools[index].ID == "homebrew.formula.git" {
					duplicate := value.Inventory.Tools[index].Installations[0]
					duplicate.ID += ":duplicate"
					value.Inventory.Tools[index].Installations = append(value.Inventory.Tools[index].Installations, duplicate)
					return
				}
			}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validFinalizeInput(t, true)
			test.mutate(&input)
			outcome, err := Finalize(input)
			if err == nil || outcome.FinalLock.ID != "" {
				t.Fatalf("unsafe Finalize() = %#v, %v", outcome, err)
			}
		})
	}
}

func validFinalizeInput(t *testing.T, installCMake bool) FinalizeInput {
	t.Helper()
	fixture := buildApplyFixture(t, installCMake)
	world := &applyWorld{}
	record, err := fixture.service(world, &applyStore{}).Execute(
		t.Context(), fixture.prepared, fixture.snapshots, fixture.confirmation,
	)
	if err != nil {
		t.Fatalf("complete fixture execution: %v", err)
	}
	finalizedAt := fixture.now.Add(time.Minute)
	current := applyInventory(finalizedAt, true)
	current.Tools = append(current.Tools, outcomeFormulaTool("git", "2.51.0", finalizedAt))
	if installCMake {
		current.Tools = append(current.Tools, outcomeFormulaTool("cmake", "4.0.3", finalizedAt))
	}
	return FinalizeInput{
		FinalizedAt: finalizedAt, Prepared: fixture.prepared, Record: record, Inventory: current,
	}
}

func outcomeFormulaTool(name, version string, at time.Time) inventory.Tool {
	source := inventory.SourceMetadata{
		Kind: inventory.SourceFixture, Name: "outcome fixture", CollectedAt: at, Confidence: inventory.ConfidenceHigh,
	}
	return inventory.Tool{
		ID: "homebrew.formula." + name, DisplayName: name, Category: inventory.CategoryUnknown,
		Installations: []inventory.Installation{{
			ID: "homebrew:formula:" + name + ":" + version, Version: version, NormalizedVersion: version,
			Path: "/opt/homebrew/Cellar/" + name + "/" + version, Architecture: inventory.ArchitectureARM64,
			Manager: "homebrew", ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			InstallReason: inventory.InstallReasonDirect, Sources: []inventory.SourceMetadata{source},
		}},
	}
}
