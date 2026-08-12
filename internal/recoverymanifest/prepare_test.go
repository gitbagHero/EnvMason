package recoverymanifest

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/defaultversion"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

type recordingRestorePreparer struct {
	child RestoreChild
	err   error
	calls []string
}

func (value *recordingRestorePreparer) PrepareRestore(
	_ context.Context,
	operationID string,
) (RestoreChild, error) {
	value.calls = append(value.calls, operationID)
	if value.err != nil {
		return RestoreChild{}, value.err
	}
	return value.child, nil
}

func TestPrepareRestoreBuildsOnlyIndependentCurrentDefaultPlan(t *testing.T) {
	manifest := mustRecoveryManifest(t)
	item, ok := findItem(
		manifest,
		recoveryOperationID('2'),
		"current-plan",
	)
	if !ok {
		t.Fatal("current plan item is missing")
	}
	restorePlan := recoveryRestorePlan(
		t,
		manifest.CreatedAt.Add(time.Minute),
		item.SourceOperationID,
		item.SourcePlanID,
	)
	sealed, err := clonePlan(restorePlan)
	if err != nil {
		t.Fatal(err)
	}
	preparer := &recordingRestorePreparer{
		child: RestoreChild{
			Plan: restorePlan,
			native: &defaultversion.Prepared{
				Plan: sealed,
			},
		},
	}
	originalManifest := cloneManifest(manifest)
	prepared, err := PrepareRestore(
		t.Context(),
		manifest,
		item.SourceOperationID,
		item.ActionID,
		preparer,
	)
	if err != nil {
		t.Fatalf("PrepareRestore() error = %v", err)
	}
	if !reflect.DeepEqual(manifest, originalManifest) {
		t.Fatal("PrepareRestore mutated the Recovery Manifest")
	}
	if !reflect.DeepEqual(
		preparer.calls,
		[]string{item.SourceOperationID},
	) {
		t.Fatalf("preparer calls = %v", preparer.calls)
	}
	if prepared.ManifestID != manifest.ID ||
		prepared.SourceOperationID != item.SourceOperationID ||
		prepared.SourcePlanID != item.SourcePlanID ||
		prepared.SourceActionID != item.ActionID ||
		prepared.Plan.ID == item.SourcePlanID ||
		prepared.Plan.SchemaVersion != plan.HighRiskExecutableSchemaVersion ||
		prepared.Plan.Actions[0].Operation != "restore_default" {
		t.Fatalf("PreparedRestore = %#v", prepared)
	}
	if !hasSourceCheck(
		prepared.Plan.Actions[0].Preconditions,
		item.SourceOperationID,
		item.SourcePlanID,
	) {
		t.Fatal("restore Plan lost its exact source binding")
	}
	originalTarget := prepared.native.Plan.Actions[0].TargetVersion
	originalBytes := *prepared.native.Plan.Actions[0].Download.Bytes
	prepared.Plan.Actions[0].TargetVersion = "99.0.0"
	*prepared.Plan.Actions[0].Download.Bytes = 99
	if prepared.native.Plan.Actions[0].TargetVersion != originalTarget ||
		*prepared.native.Plan.Actions[0].Download.Bytes != originalBytes {
		t.Fatal("public restore Plan mutation changed sealed native context")
	}
}

func TestPrepareRestoreRejectsEveryNonWhitelistDispositionBeforePreparer(t *testing.T) {
	manifest := mustRecoveryManifest(t)
	tests := []string{
		"current-manual",
		"drifted-plan",
		"no-verifier",
		"uncertain",
	}
	preparer := &recordingRestorePreparer{}
	for _, actionID := range tests {
		if _, err := PrepareRestore(
			t.Context(),
			manifest,
			recoveryOperationID('2'),
			actionID,
			preparer,
		); err == nil {
			t.Fatalf("non-whitelist item %q was accepted", actionID)
		}
	}
	if _, err := PrepareRestore(
		t.Context(),
		manifest,
		recoveryOperationID('1'),
		"missing",
		preparer,
	); err == nil {
		t.Fatal("missing item was accepted")
	}
	if len(preparer.calls) != 0 {
		t.Fatalf("non-whitelist item reached preparer: %v", preparer.calls)
	}
}

func TestPrepareRestoreRejectsNonDefaultPlanCandidate(t *testing.T) {
	reviews := recoveryReviews()
	reviews[0].Candidates[1].Operation = "install_version"
	manifest, err := Build(
		time.Date(2026, 7, 31, 10, 0, 0, 0, time.UTC),
		reviews,
	)
	if err != nil {
		t.Fatal(err)
	}
	preparer := &recordingRestorePreparer{}
	if _, err := PrepareRestore(
		t.Context(),
		manifest,
		recoveryOperationID('2'),
		"current-plan",
		preparer,
	); err == nil {
		t.Fatal("non-default plan candidate was accepted")
	}
	if len(preparer.calls) != 0 {
		t.Fatal("non-default candidate reached restore preparer")
	}
}

func TestPrepareRestoreRejectsCancelledContextBeforePreparer(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	preparer := &recordingRestorePreparer{}
	if _, err := PrepareRestore(
		ctx,
		mustRecoveryManifest(t),
		recoveryOperationID('2'),
		"current-plan",
		preparer,
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation error = %v", err)
	}
	if len(preparer.calls) != 0 {
		t.Fatal("cancelled request reached restore preparer")
	}
}

func TestPrepareRestoreRejectsStaleWrongOrMismatchedPlans(t *testing.T) {
	manifest := mustRecoveryManifest(t)
	item, _ := findItem(
		manifest,
		recoveryOperationID('2'),
		"current-plan",
	)
	tests := []struct {
		name string
		plan plan.Plan
	}{
		{
			name: "stale",
			plan: recoveryRestorePlan(
				t,
				manifest.CreatedAt.Add(-time.Nanosecond),
				item.SourceOperationID,
				item.SourcePlanID,
			),
		},
		{
			name: "wrong operation",
			plan: recoveryDefaultSetPlan(
				t,
				manifest.CreatedAt.Add(time.Minute),
			),
		},
		{
			name: "wrong source",
			plan: recoveryRestorePlan(
				t,
				manifest.CreatedAt.Add(time.Minute),
				recoveryOperationID('f'),
				recoveryDigest('f'),
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preparer := &recordingRestorePreparer{
				child: RestoreChild{Plan: test.plan},
			}
			if _, err := PrepareRestore(
				t.Context(),
				manifest,
				item.SourceOperationID,
				item.ActionID,
				preparer,
			); err == nil {
				t.Fatal("PrepareRestore unexpectedly succeeded")
			}
			if len(preparer.calls) != 1 {
				t.Fatalf("preparer calls = %v", preparer.calls)
			}
		})
	}

	preparer := &recordingRestorePreparer{
		err: errors.New("source default drifted"),
	}
	if _, err := PrepareRestore(
		t.Context(),
		manifest,
		item.SourceOperationID,
		item.ActionID,
		preparer,
	); err == nil || err.Error() != "source default drifted" {
		t.Fatalf("preparer error = %v", err)
	}
}

func TestPrepareRestoreRejectsMismatchedNativeContext(t *testing.T) {
	manifest := mustRecoveryManifest(t)
	item, _ := findItem(
		manifest,
		recoveryOperationID('2'),
		"current-plan",
	)
	restorePlan := recoveryRestorePlan(
		t,
		manifest.CreatedAt.Add(time.Minute),
		item.SourceOperationID,
		item.SourcePlanID,
	)
	wrong := recoveryRestorePlan(
		t,
		manifest.CreatedAt.Add(time.Minute),
		recoveryOperationID('f'),
		recoveryDigest('f'),
	)
	preparer := &recordingRestorePreparer{
		child: RestoreChild{
			Plan: restorePlan,
			native: &defaultversion.Prepared{
				Plan: wrong,
			},
		},
	}
	if _, err := PrepareRestore(
		t.Context(),
		manifest,
		item.SourceOperationID,
		item.ActionID,
		preparer,
	); err == nil {
		t.Fatal("mismatched native restore context was accepted")
	}
}

func recoveryRestorePlan(
	t *testing.T,
	createdAt time.Time,
	sourceOperationID string,
	sourcePlanID string,
) plan.Plan {
	t.Helper()
	value, err := plan.BuildDefaultRestore(plan.DefaultRestoreInput{
		Inventory:              recoveryDefaultInventory(createdAt),
		CreatedAt:              createdAt,
		ScriptDigest:           recoveryDigest('c'),
		CurrentAliasDigest:     recoveryAliasDigest("v24.14.0"),
		CurrentAlias:           "v24.14.0",
		CurrentDefaultVersion:  "v24.14.0",
		OriginalAliasDigest:    recoveryAliasDigest("22"),
		OriginalAlias:          "22",
		OriginalDefaultVersion: "v22.0.0",
		SourceOperationID:      sourceOperationID,
		SourcePlanID:           sourcePlanID,
	})
	if err != nil {
		t.Fatalf("BuildDefaultRestore() error = %v", err)
	}
	return value
}

func recoveryDefaultSetPlan(
	t *testing.T,
	createdAt time.Time,
) plan.Plan {
	t.Helper()
	value, err := plan.BuildDefaultSet(plan.DefaultSetInput{
		Inventory:             recoveryDefaultInventory(createdAt),
		CreatedAt:             createdAt,
		TargetVersion:         "24.14.0",
		ScriptDigest:          recoveryDigest('c'),
		CurrentAliasDigest:    recoveryAliasDigest("22"),
		CurrentAlias:          "22",
		CurrentDefaultVersion: "v22.0.0",
	})
	if err != nil {
		t.Fatalf("BuildDefaultSet() error = %v", err)
	}
	return value
}

func recoveryDefaultInventory(generatedAt time.Time) inventory.Inventory {
	return inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion,
		GeneratedAt:   generatedAt,
		System: inventory.System{
			OS: inventory.OSMacOS, OSVersion: "15.0",
			Architecture: inventory.ArchitectureARM64,
		},
		Tools: []inventory.Tool{{
			ID: "runtime.node", DisplayName: "Node.js",
			Category: inventory.CategoryRuntime,
			Installations: []inventory.Installation{
				{
					ID: "node-nvm-22", Version: "v22.0.0",
					Path:    "$HOME/.nvm/versions/node/v22.0.0/bin/node",
					Manager: "nvm", Architecture: inventory.ArchitectureARM64,
					ActiveState:  inventory.ActiveStateActive,
					DefaultState: inventory.DefaultStateNonDefault,
				},
				{
					ID: "node-nvm-24", Version: "v24.14.0",
					Path:    "$HOME/.nvm/versions/node/v24.14.0/bin/node",
					Manager: "nvm", Architecture: inventory.ArchitectureARM64,
					ActiveState:  inventory.ActiveStateInactive,
					DefaultState: inventory.DefaultStateDefault,
				},
			},
		}},
	}
}

func recoveryAliasDigest(value string) string {
	digest := sha256.Sum256([]byte(value + "\n"))
	return "sha256:" + hex.EncodeToString(digest[:])
}
