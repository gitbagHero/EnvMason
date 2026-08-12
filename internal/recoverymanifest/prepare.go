package recoverymanifest

import (
	"context"
	"errors"

	"github.com/gitbagHero/EnvMason/internal/defaultversion"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

// RestorePreparer delegates the one allowed recovery case to the existing
// defaultversion service. It prepares but never confirms or executes a Plan.
type RestorePreparer interface {
	PrepareRestore(context.Context, string) (RestoreChild, error)
}

// RestoreChild contains a newly prepared existing R3 restore Plan.
type RestoreChild struct {
	Plan   plan.Plan
	native *defaultversion.Prepared
}

// NativeRestorePreparer adapts defaultversion.PrepareRestore without
// reimplementing its source loading, fresh scan or drift checks.
type NativeRestorePreparer struct {
	Service defaultversion.Service
}

func (value NativeRestorePreparer) PrepareRestore(
	ctx context.Context,
	operationID string,
) (RestoreChild, error) {
	prepared, err := value.Service.PrepareRestore(
		ctx,
		defaultversion.RestoreOptions{OperationID: operationID},
	)
	if err != nil {
		return RestoreChild{}, err
	}
	copied, err := clonePlan(prepared.Plan)
	if err != nil {
		return RestoreChild{}, err
	}
	return RestoreChild{Plan: copied, native: &prepared}, nil
}

// PreparedRestore binds a new R3 restore Plan to the exact review-only
// Recovery Manifest item that allowed it to be prepared.
type PreparedRestore struct {
	ManifestID        string
	SourceOperationID string
	SourcePlanID      string
	SourceActionID    string
	Plan              plan.Plan
	native            *defaultversion.Prepared
}

// PrepareRestore looks up one immutable Manifest item and only delegates
// changed/current/plan NVM set-default candidates to the existing R3 builder.
func PrepareRestore(
	ctx context.Context,
	manifest Manifest,
	sourceOperationID string,
	actionID string,
	preparer RestorePreparer,
) (PreparedRestore, error) {
	if err := Validate(manifest); err != nil {
		return PreparedRestore{}, errors.New(
			"prepare Recovery Plan: Recovery Manifest is invalid",
		)
	}
	if err := ctx.Err(); err != nil {
		return PreparedRestore{}, err
	}
	if preparer == nil {
		return PreparedRestore{}, errors.New(
			"prepare Recovery Plan: restore preparer is required",
		)
	}
	item, ok := findItem(manifest, sourceOperationID, actionID)
	if !ok {
		return PreparedRestore{}, errors.New(
			"prepare Recovery Plan: Manifest item was not found",
		)
	}
	if !restorableDefaultItem(item) {
		return PreparedRestore{}, errors.New(
			"prepare Recovery Plan: item is not a current plan-mode NVM default candidate",
		)
	}
	child, err := preparer.PrepareRestore(ctx, item.SourceOperationID)
	if err != nil {
		return PreparedRestore{}, err
	}
	if err := validateRestorePlan(child.Plan, manifest, item); err != nil {
		return PreparedRestore{}, err
	}
	if child.native != nil &&
		(plan.Validate(child.native.Plan) != nil ||
			child.native.Plan.ID != child.Plan.ID) {
		return PreparedRestore{}, errors.New(
			"prepare Recovery Plan: native context does not match the restore Plan",
		)
	}
	copied, err := clonePlan(child.Plan)
	if err != nil {
		return PreparedRestore{}, err
	}
	return PreparedRestore{
		ManifestID:        manifest.ID,
		SourceOperationID: item.SourceOperationID,
		SourcePlanID:      item.SourcePlanID,
		SourceActionID:    item.ActionID,
		Plan:              copied,
		native:            child.native,
	}, nil
}

func findItem(
	manifest Manifest,
	sourceOperationID string,
	actionID string,
) (Item, bool) {
	for _, item := range manifest.Items {
		if item.SourceOperationID == sourceOperationID &&
			item.ActionID == actionID {
			return item, true
		}
	}
	return Item{}, false
}

func restorableDefaultItem(value Item) bool {
	return value.ToolID == "runtime.node" &&
		value.Operation == "set_default" &&
		value.Evidence == execution.RecoveryEvidenceChanged &&
		value.CurrentState == execution.RecoveryCheckpointCurrent &&
		value.RecoveryMode == "plan" &&
		value.Disposition == DispositionPrepareNewPlan
}

func validateRestorePlan(
	value plan.Plan,
	manifest Manifest,
	item Item,
) error {
	if err := plan.Validate(value); err != nil {
		return errors.New("prepare Recovery Plan: restore Plan is invalid")
	}
	if value.SchemaVersion != plan.HighRiskExecutableSchemaVersion ||
		!value.Executable ||
		value.Continuation != nil ||
		value.CreatedAt.Before(manifest.CreatedAt) ||
		value.ID == item.SourcePlanID ||
		len(value.Actions) != 1 {
		return errors.New(
			"prepare Recovery Plan: a new executable Plan 0.3.0 is required",
		)
	}
	action := value.Actions[0]
	if action.ID != "restore-node-default" ||
		action.ToolID != "runtime.node" ||
		action.Operation != "restore_default" ||
		action.Adapter != "nvm" ||
		action.Risk != plan.RiskR3 ||
		!action.Confirmation.Required ||
		action.Confirmation.Scope != "plan" ||
		action.Recovery.Mode != "manual" ||
		!hasSourceCheck(
			action.Preconditions,
			item.SourceOperationID,
			item.SourcePlanID,
		) {
		return errors.New(
			"prepare Recovery Plan: restore action does not match its source",
		)
	}
	return nil
}

func hasSourceCheck(
	values []plan.Check,
	operationID string,
	planID string,
) bool {
	for _, value := range values {
		if value.Kind == "source_operation_matches" &&
			value.Subject == operationID &&
			value.Expected == planID {
			return true
		}
	}
	return false
}

func clonePlan(value plan.Plan) (plan.Plan, error) {
	data, err := plan.Marshal(value)
	if err != nil {
		return plan.Plan{}, err
	}
	return plan.Decode(data)
}
