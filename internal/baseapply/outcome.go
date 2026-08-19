package baseapply

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"time"

	"github.com/gitbagHero/EnvMason/internal/baseinstall"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
)

// FinalizeInput contains only explicit post-execution evidence. Finalize does
// not discover the machine, read history or write the resulting Lock.
type FinalizeInput struct {
	FinalizedAt time.Time
	Prepared    Prepared
	Record      execution.Record
	Inventory   inventory.Inventory
}

// Outcome is the internal I21-E result. It intentionally omits raw Inventory,
// paths, commands, environment values and process output.
type Outcome struct {
	OperationID string        `json:"operation_id"`
	PlanID      string        `json:"plan_id"`
	ReviewID    string        `json:"review_id"`
	FinalLock   lockfile.Lock `json:"final_lock"`
	Diff        LockDiff      `json:"diff"`
}

type LockDiff struct {
	PreviousLockID string       `json:"previous_lock_id"`
	FinalLockID    string       `json:"final_lock_id"`
	Changes        []LockChange `json:"changes"`
}

type LockChange struct {
	ItemID      string                   `json:"item_id"`
	Capability  string                   `json:"capability"`
	ToolID      string                   `json:"tool_id"`
	Manager     string                   `json:"manager"`
	Version     string                   `json:"version"`
	BeforeState lockfile.ResolutionState `json:"before_state"`
	AfterState  lockfile.ResolutionState `json:"after_state"`
}

// Finalize derives the final Base Lock only from one fully completed I21-D
// operation and a same-time explicit Inventory that still proves the complete
// reviewed formula closure. Partial or failed execution never yields a Lock.
func Finalize(input FinalizeInput) (Outcome, error) {
	finalizedAt := input.FinalizedAt.UTC()
	if input.FinalizedAt.IsZero() {
		return Outcome{}, errors.New("finalize Base Homebrew execution: finalized_at is required")
	}
	if err := validatePreparedOutcome(input.Prepared); err != nil {
		return Outcome{}, err
	}
	if err := validateCompletedRecord(input.Prepared, input.Record, finalizedAt); err != nil {
		return Outcome{}, err
	}
	if err := validatePostInventory(input.Prepared, input.Inventory, finalizedAt); err != nil {
		return Outcome{}, err
	}
	if err := baseinstall.ValidateInstalledTransactionClosure(input.Prepared.Review, input.Inventory); err != nil {
		return Outcome{}, errors.New("finalize Base Homebrew execution: post-execution formula closure is not exact")
	}
	if err := validateRecordedClosure(input.Prepared, input.Record); err != nil {
		return Outcome{}, err
	}

	updates, changes, err := outcomeStateChanges(input.Prepared)
	if err != nil {
		return Outcome{}, err
	}
	finalLock, err := lockfile.DeriveState(input.Prepared.Lock, finalizedAt, updates)
	if err != nil {
		return Outcome{}, errors.New("finalize Base Homebrew execution: derive final Lock")
	}
	return Outcome{
		OperationID: input.Record.ID,
		PlanID:      input.Prepared.Plan.ID,
		ReviewID:    input.Prepared.Review.ID,
		FinalLock:   finalLock,
		Diff: LockDiff{
			PreviousLockID: input.Prepared.Lock.ID,
			FinalLockID:    finalLock.ID,
			Changes:        changes,
		},
	}, nil
}

func validatePreparedOutcome(value Prepared) error {
	want, err := Prepare(value.CandidatePlan, value.Lock, value.Review)
	if err != nil || !reflect.DeepEqual(want, value) {
		return errors.New("finalize Base Homebrew execution: Prepared binding is invalid")
	}
	return nil
}

func validateCompletedRecord(prepared Prepared, record execution.Record, finalizedAt time.Time) error {
	if execution.ValidateRecord(record) != nil || record.SchemaVersion != execution.RecordSchemaVersion ||
		record.State != execution.StateCompleted || record.FinishedAt == nil ||
		finalizedAt.Before(*record.FinishedAt) || record.PlanID != prepared.Plan.ID ||
		record.ConfirmedPlan == nil || !reflect.DeepEqual(*record.ConfirmedPlan, prepared.Plan) {
		return errors.New("finalize Base Homebrew execution: one completed bound Operation Record is required")
	}
	return nil
}

func validatePostInventory(prepared Prepared, current inventory.Inventory, finalizedAt time.Time) error {
	if _, err := inventory.Marshal(current); err != nil || current.SchemaVersion != inventory.SchemaVersion ||
		!current.GeneratedAt.Equal(finalizedAt) || current.System.OS != prepared.Lock.Target.OS ||
		current.System.OSVersion != prepared.Lock.Target.OSVersion ||
		current.System.Architecture != prepared.Lock.Target.Architecture {
		return errors.New("finalize Base Homebrew execution: current post-execution Inventory is invalid")
	}
	found := 0
	for _, tool := range current.Tools {
		if tool.ID != "manager.homebrew" {
			continue
		}
		for _, installation := range tool.Installations {
			if installation.ActiveState != inventory.ActiveStateActive {
				continue
			}
			found++
			if installation.Manager != "homebrew" ||
				installation.ID != prepared.Plan.Environment.ActiveInstallationID ||
				installation.Version != prepared.Plan.Environment.ActiveVersion ||
				installation.Path != activeHomebrewPathForOutcome(prepared) ||
				installation.Architecture != prepared.Lock.Target.Architecture {
				return errors.New("finalize Base Homebrew execution: active Homebrew identity changed")
			}
		}
	}
	if found != 1 {
		return errors.New("finalize Base Homebrew execution: exactly one reviewed active Homebrew installation is required")
	}
	return nil
}

func activeHomebrewPathForOutcome(prepared Prepared) string {
	for _, installation := range prepared.Plan.Environment.Installations {
		if installation.ID == prepared.Plan.Environment.ActiveInstallationID {
			return installation.Path
		}
	}
	return ""
}

func validateRecordedClosure(prepared Prepared, record execution.Record) error {
	previews := make(map[string]baseinstall.ActionPreview, len(prepared.Review.Actions))
	for _, preview := range prepared.Review.Actions {
		previews[preview.ActionID] = preview
	}
	if len(record.Steps) != len(previews) {
		return errors.New("finalize Base Homebrew execution: Operation Record lacks reviewed state evidence")
	}
	for _, step := range record.Steps {
		preview, exists := previews[step.ActionID]
		if !exists || step.Before == nil || step.After == nil {
			return errors.New("finalize Base Homebrew execution: Operation Record lacks reviewed state evidence")
		}
		if !validOutcomeBeforeFacts(prepared, preview, step.Before.Facts) {
			return errors.New("finalize Base Homebrew execution: Operation Record initial closure does not match the Review")
		}
		expected, err := execution.NewSnapshot(expectedOutcomeFacts(prepared, preview))
		if err != nil || !reflect.DeepEqual(*step.After, expected) {
			return errors.New("finalize Base Homebrew execution: Operation Record final closure does not match the Review")
		}
	}
	return nil
}

func validOutcomeBeforeFacts(
	prepared Prepared,
	preview baseinstall.ActionPreview,
	facts map[string]string,
) bool {
	expected := expectedOutcomeFacts(prepared, preview)
	if len(facts) != len(expected) || facts["action_id"] != preview.ActionID ||
		facts["lock_id"] != prepared.Review.LockID || facts["plan_id"] != prepared.Plan.ID ||
		facts["review_id"] != prepared.Review.ID {
		return false
	}
	formulae := append([]baseinstall.FormulaPreview{preview.Root}, preview.Dependencies...)
	for _, formula := range formulae {
		observed := facts["formula."+formula.Name]
		if observed != formula.Version &&
			(formula.State != baseinstall.FormulaInstallRequired || observed != "absent") {
			return false
		}
	}
	return true
}

func expectedOutcomeFacts(prepared Prepared, preview baseinstall.ActionPreview) map[string]string {
	facts := map[string]string{
		"action_id": preview.ActionID,
		"lock_id":   prepared.Review.LockID,
		"plan_id":   prepared.Plan.ID,
		"review_id": prepared.Review.ID,
	}
	formulae := append([]baseinstall.FormulaPreview{preview.Root}, preview.Dependencies...)
	for _, formula := range formulae {
		facts["formula."+formula.Name] = formula.Version
	}
	return facts
}

func outcomeStateChanges(prepared Prepared) ([]lockfile.StateUpdate, []LockChange, error) {
	items := make(map[string]lockfile.Item, len(prepared.Lock.Items))
	for _, item := range prepared.Lock.Items {
		items[item.Capability] = item
	}
	updates := make([]lockfile.StateUpdate, 0, len(prepared.Review.Actions))
	changes := make([]LockChange, 0, len(prepared.Review.Actions))
	for _, preview := range prepared.Review.Actions {
		capability := "base." + preview.Root.Name
		item, exists := items[capability]
		if !exists || item.Module != "base" || item.State != lockfile.StateInstallRequired ||
			item.Implementation == nil || item.Implementation.ToolID != "homebrew.formula."+preview.Root.Name ||
			item.Implementation.Manager != "homebrew" || item.Implementation.PackageKind != "formula" ||
			item.Implementation.PackageID != preview.Root.Name || item.Implementation.Version != preview.Root.Version {
			return nil, nil, fmt.Errorf("finalize Base Homebrew execution: Lock item %q does not match its reviewed action", capability)
		}
		updates = append(updates, lockfile.StateUpdate{
			ItemID: item.ID, State: lockfile.StateSatisfied,
			Observed: []lockfile.Observation{{Manager: "homebrew", Version: preview.Root.Version}},
			Reason:   lockfile.ReasonCompatibleInstallation,
		})
		changes = append(changes, LockChange{
			ItemID: item.ID, Capability: item.Capability,
			ToolID: item.Implementation.ToolID, Manager: "homebrew", Version: preview.Root.Version,
			BeforeState: lockfile.StateInstallRequired, AfterState: lockfile.StateSatisfied,
		})
	}
	sort.Slice(updates, func(left, right int) bool { return updates[left].ItemID < updates[right].ItemID })
	sort.Slice(changes, func(left, right int) bool { return changes[left].ItemID < changes[right].ItemID })
	return updates, changes, nil
}
