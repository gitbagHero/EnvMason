package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/apply"
	"github.com/gitbagHero/EnvMason/internal/assessment"
	"github.com/gitbagHero/EnvMason/internal/defaultversion"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

type recordingStagePreparer struct {
	installPlan ChildPlan
	defaultPlan ChildPlan
	toolsPlan   ChildPlan
	failStage   string
	calls       []string
	nodeTargets []string
	toolTargets []NodeToolTargets
}

func (value *recordingStagePreparer) PrepareInstallNode(
	_ context.Context,
	nodeVersion string,
) (ChildPlan, error) {
	value.calls = append(value.calls, StageInstallNode)
	value.nodeTargets = append(value.nodeTargets, nodeVersion)
	if value.failStage == StageInstallNode {
		return ChildPlan{}, errors.New("install preparation failed")
	}
	return value.installPlan, nil
}

func (value *recordingStagePreparer) PrepareSetDefault(
	_ context.Context,
	nodeVersion string,
) (ChildPlan, error) {
	value.calls = append(value.calls, StageSetDefault)
	value.nodeTargets = append(value.nodeTargets, nodeVersion)
	if value.failStage == StageSetDefault {
		return ChildPlan{}, errors.New("default preparation failed")
	}
	return value.defaultPlan, nil
}

func (value *recordingStagePreparer) PrepareNodeTools(
	_ context.Context,
	nodeVersion string,
	targets NodeToolTargets,
) (ChildPlan, error) {
	value.calls = append(value.calls, StageUpdateNodeTools)
	value.nodeTargets = append(value.nodeTargets, nodeVersion)
	value.toolTargets = append(value.toolTargets, targets)
	if value.failStage == StageUpdateNodeTools {
		return ChildPlan{}, errors.New("tools preparation failed")
	}
	return value.toolsPlan, nil
}

func TestPrepareNextPreparesAndBindsOnlyTheReadyStage(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	preparer := &recordingStagePreparer{
		installPlan: workflowChild(
			workflowInstallPlan(t, record.UpdatedAt.Add(time.Minute), "24.14.0"),
		),
	}
	originalManifest := cloneManifest(manifest)
	originalRecord := cloneRecord(record)

	install, err := PrepareNext(t.Context(), manifest, record, preparer)
	if err != nil {
		t.Fatalf("PrepareNext(install) error = %v", err)
	}
	if install.StageID != StageInstallNode ||
		install.Plan.SchemaVersion != plan.ExecutableSchemaVersion ||
		install.Record.Stages[0].State != StagePlanned ||
		install.Record.Stages[0].PlanID != install.Plan.ID {
		t.Fatalf("install preparation = %#v", install)
	}
	if !reflect.DeepEqual(manifest, originalManifest) ||
		!reflect.DeepEqual(record, originalRecord) {
		t.Fatal("PrepareNext mutated its Manifest or Record input")
	}
	if len(preparer.calls) != 1 || preparer.calls[0] != StageInstallNode {
		t.Fatalf("preparer calls = %v", preparer.calls)
	}
	if _, err := PrepareNext(t.Context(), manifest, install.Record, preparer); err == nil {
		t.Fatal("planned stage allowed another preparation")
	}
	if len(preparer.calls) != 1 {
		t.Fatalf("non-Ready Record reached preparer: %v", preparer.calls)
	}

	record = completePreparedStage(t, manifest, install, 0)
	preparer.defaultPlan = workflowChild(
		workflowDefaultPlan(t, record.UpdatedAt.Add(time.Minute), "24.14.0"),
	)
	setDefault, err := PrepareNext(t.Context(), manifest, record, preparer)
	if err != nil {
		t.Fatalf("PrepareNext(default) error = %v", err)
	}
	if setDefault.StageID != StageSetDefault ||
		setDefault.Plan.SchemaVersion != plan.HighRiskExecutableSchemaVersion ||
		setDefault.Record.Stages[1].State != StagePlanned ||
		setDefault.Record.Stages[1].PlanID != setDefault.Plan.ID {
		t.Fatalf("default preparation = %#v", setDefault)
	}

	record = completePreparedStage(t, manifest, setDefault, 1)
	preparer.toolsPlan = workflowChild(
		workflowNodeToolsPlan(t, record.UpdatedAt.Add(time.Minute), "24.14.0", manifest.NodeTools),
	)
	tools, err := PrepareNext(t.Context(), manifest, record, preparer)
	if err != nil {
		t.Fatalf("PrepareNext(tools) error = %v", err)
	}
	if tools.StageID != StageUpdateNodeTools ||
		tools.Plan.SchemaVersion != plan.ExecutableSchemaVersion ||
		tools.Record.Stages[2].State != StagePlanned ||
		tools.Record.Stages[2].PlanID != tools.Plan.ID {
		t.Fatalf("Node tools preparation = %#v", tools)
	}
	if !reflect.DeepEqual(
		preparer.calls,
		[]string{StageInstallNode, StageSetDefault, StageUpdateNodeTools},
	) {
		t.Fatalf("preparer calls = %v", preparer.calls)
	}
	if !reflect.DeepEqual(
		preparer.nodeTargets,
		[]string{"24.14.0", "24.14.0", "24.14.0"},
	) {
		t.Fatalf("node targets = %v", preparer.nodeTargets)
	}
	if len(preparer.toolTargets) != 1 ||
		preparer.toolTargets[0] != manifest.NodeTools {
		t.Fatalf("tool targets = %#v", preparer.toolTargets)
	}

	record = completePreparedStage(t, manifest, tools, 2)
	if _, err := PrepareNext(t.Context(), manifest, record, preparer); err == nil {
		t.Fatal("completed workflow allowed another preparation")
	}
	if len(preparer.calls) != 3 {
		t.Fatalf("completed workflow reached preparer: %v", preparer.calls)
	}
}

func TestPrepareNextRejectsFailureAndAbnormalTerminalWithoutMutation(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	preparer := &recordingStagePreparer{
		failStage: StageInstallNode,
	}
	original := cloneRecord(record)
	if _, err := PrepareNext(t.Context(), manifest, record, preparer); err == nil ||
		!strings.Contains(err.Error(), "install preparation failed") {
		t.Fatalf("preparation error = %v", err)
	}
	if !reflect.DeepEqual(record, original) {
		t.Fatal("failed preparation mutated its Record")
	}

	planned, err := BindStagePlan(
		record,
		manifest,
		StageInstallNode,
		digestID('1'),
		record.UpdatedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	running, err := StartStage(
		planned,
		manifest,
		StageInstallNode,
		operationID('1'),
		planned.UpdatedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	failed, err := FinishStage(
		running,
		manifest,
		StageInstallNode,
		StageFailed,
		"",
		running.UpdatedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	calls := len(preparer.calls)
	if _, err := PrepareNext(t.Context(), manifest, failed, preparer); err == nil {
		t.Fatal("failed workflow allowed a later Plan")
	}
	if len(preparer.calls) != calls {
		t.Fatal("terminal workflow reached a stage preparer")
	}
}

func TestPrepareNextRejectsStaleOrMismatchedChildPlans(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	tests := []struct {
		name string
		plan plan.Plan
	}{
		{
			name: "stale",
			plan: workflowInstallPlan(
				t,
				record.UpdatedAt.Add(-time.Nanosecond),
				"24.14.0",
			),
		},
		{
			name: "wrong stage",
			plan: workflowDefaultPlan(
				t,
				record.UpdatedAt.Add(time.Minute),
				"24.14.0",
			),
		},
		{
			name: "wrong target",
			plan: workflowInstallPlan(
				t,
				record.UpdatedAt.Add(time.Minute),
				"24.15.0",
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			preparer := &recordingStagePreparer{
				installPlan: workflowChild(test.plan),
			}
			original := cloneRecord(record)
			if _, err := PrepareNext(t.Context(), manifest, record, preparer); err == nil {
				t.Fatal("PrepareNext unexpectedly succeeded")
			}
			if !reflect.DeepEqual(record, original) {
				t.Fatal("rejected Plan mutated its Record")
			}
			if len(preparer.calls) != 1 {
				t.Fatalf("preparer calls = %v", preparer.calls)
			}
		})
	}
}

func TestPrepareNextRejectsNodeToolsBoundToAnotherNode(t *testing.T) {
	manifest := mustManifest(t)
	record := readyAtStage(t, manifest, 2)
	preparer := &recordingStagePreparer{
		toolsPlan: workflowChild(
			workflowNodeToolsPlan(
				t,
				record.UpdatedAt.Add(time.Minute),
				"24.15.0",
				manifest.NodeTools,
			),
		),
	}
	original := cloneRecord(record)
	if _, err := PrepareNext(t.Context(), manifest, record, preparer); err == nil {
		t.Fatal("Node tools Plan for another Node version was accepted")
	}
	if !reflect.DeepEqual(record, original) {
		t.Fatal("rejected Node tools Plan mutated its Record")
	}
}

func TestPrepareNextRejectsWrongTargetsForLaterStages(t *testing.T) {
	manifest := mustManifest(t)
	defaultRecord := readyAtStage(t, manifest, 1)
	defaultPreparer := &recordingStagePreparer{
		defaultPlan: workflowChild(
			workflowDefaultPlan(
				t,
				defaultRecord.UpdatedAt.Add(time.Minute),
				"24.15.0",
			),
		),
	}
	if _, err := PrepareNext(
		t.Context(),
		manifest,
		defaultRecord,
		defaultPreparer,
	); err == nil {
		t.Fatal("default Plan with the wrong Node target was accepted")
	}

	toolsRecord := readyAtStage(t, manifest, 2)
	wrongTargets := manifest.NodeTools
	wrongTargets.NPM = "12.0.2"
	toolsPreparer := &recordingStagePreparer{
		toolsPlan: workflowChild(
			workflowNodeToolsPlan(
				t,
				toolsRecord.UpdatedAt.Add(time.Minute),
				manifest.TargetNodeVersion,
				wrongTargets,
			),
		),
	}
	if _, err := PrepareNext(
		t.Context(),
		manifest,
		toolsRecord,
		toolsPreparer,
	); err == nil {
		t.Fatal("Node tools Plan with the wrong tool target was accepted")
	}
}

func TestPrepareNextRejectsCancelledContextBeforePreparer(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	preparer := &recordingStagePreparer{}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := PrepareNext(ctx, manifest, record, preparer); !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf("PrepareNext cancellation error = %v", err)
	}
	if len(preparer.calls) != 0 {
		t.Fatal("cancelled context reached stage preparer")
	}
}

func TestPrepareNextDoesNotSharePublicPlanWithNativeContext(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	childPlan := workflowInstallPlan(
		t,
		record.UpdatedAt.Add(time.Minute),
		manifest.TargetNodeVersion,
	)
	sealed := apply.Prepared{Plan: clonePlan(childPlan)}
	preparer := &recordingStagePreparer{
		installPlan: ChildPlan{
			Plan: childPlan,
			native: &nativePrepared{
				install: &sealed,
			},
		},
	}
	prepared, err := PrepareNext(t.Context(), manifest, record, preparer)
	if err != nil {
		t.Fatal(err)
	}
	originalID := prepared.native.install.Plan.ID
	originalTarget := prepared.native.install.Plan.Actions[0].TargetVersion
	prepared.Plan.ID = digestID('f')
	prepared.Plan.Actions[0].TargetVersion = "99.0.0"
	prepared.Plan.Actions[0].Preconditions[0].Expected = "changed"
	if prepared.native.install.Plan.ID != originalID ||
		prepared.native.install.Plan.Actions[0].TargetVersion != originalTarget ||
		prepared.native.install.Plan.Actions[0].Preconditions[0].Expected == "changed" {
		t.Fatal("public Plan mutation changed the sealed native context")
	}

	record = readyAtStage(t, manifest, 1)
	defaultPlan := workflowDefaultPlan(
		t,
		record.UpdatedAt.Add(time.Minute),
		manifest.TargetNodeVersion,
	)
	sealedDefault := defaultversion.Prepared{Plan: clonePlan(defaultPlan)}
	preparer = &recordingStagePreparer{
		defaultPlan: ChildPlan{
			Plan: defaultPlan,
			native: &nativePrepared{
				setDefault: &sealedDefault,
			},
		},
	}
	prepared, err = PrepareNext(t.Context(), manifest, record, preparer)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Plan.Actions[0].Download.Bytes == nil ||
		prepared.native.setDefault.Plan.Actions[0].Download.Bytes == nil {
		t.Fatal("default Plan does not expose its zero-byte download evidence")
	}
	*prepared.Plan.Actions[0].Download.Bytes = 99
	if *prepared.native.setDefault.Plan.Actions[0].Download.Bytes != 0 {
		t.Fatal("public Download pointer mutation changed the native context")
	}
}

func TestPrepareNextRejectsMismatchedNativeContext(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	childPlan := workflowInstallPlan(
		t,
		record.UpdatedAt.Add(time.Minute),
		manifest.TargetNodeVersion,
	)
	defaultPlan := workflowDefaultPlan(
		t,
		record.UpdatedAt.Add(time.Minute),
		manifest.TargetNodeVersion,
	)
	preparer := &recordingStagePreparer{
		installPlan: ChildPlan{
			Plan: childPlan,
			native: &nativePrepared{
				setDefault: &defaultversion.Prepared{Plan: defaultPlan},
			},
		},
	}
	if _, err := PrepareNext(t.Context(), manifest, record, preparer); err == nil {
		t.Fatal("wrong native context variant was accepted")
	}
}

func completePreparedStage(
	t *testing.T,
	manifest Manifest,
	prepared PreparedStage,
	index int,
) Record {
	t.Helper()
	running, err := StartStage(
		prepared.Record,
		manifest,
		prepared.StageID,
		operationID(byte('1'+index)),
		prepared.Record.UpdatedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("StartStage(%d) error = %v", index, err)
	}
	finished, err := FinishStage(
		running,
		manifest,
		prepared.StageID,
		StageCompleted,
		digestID(byte('a'+index)),
		running.UpdatedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("FinishStage(%d) error = %v", index, err)
	}
	return finished
}

func readyAtStage(t *testing.T, manifest Manifest, target int) Record {
	t.Helper()
	record := mustNewRecord(t, manifest)
	for index := 0; index < target; index++ {
		var child plan.Plan
		switch index {
		case 0:
			child = workflowInstallPlan(
				t,
				record.UpdatedAt.Add(time.Minute),
				manifest.TargetNodeVersion,
			)
		case 1:
			child = workflowDefaultPlan(
				t,
				record.UpdatedAt.Add(time.Minute),
				manifest.TargetNodeVersion,
			)
		}
		prepared, err := PrepareNext(
			t.Context(),
			manifest,
			record,
			&recordingStagePreparer{
				installPlan: workflowChild(child),
				defaultPlan: workflowChild(child),
			},
		)
		if err != nil {
			t.Fatalf("PrepareNext(%d) error = %v", index, err)
		}
		record = completePreparedStage(t, manifest, prepared, index)
	}
	return record
}

func workflowChild(value plan.Plan) ChildPlan {
	return ChildPlan{Plan: value}
}

func workflowInstallPlan(
	t *testing.T,
	createdAt time.Time,
	targetVersion string,
) plan.Plan {
	t.Helper()
	policy := assessment.DefaultPolicy()
	policy.Tools["runtime.node"] = assessment.ToolPolicy{
		Channel: assessment.ChannelLTS,
		Pin:     targetVersion,
	}
	value, err := plan.BuildExecutable(plan.BuildInput{
		Inventory: workflowInventory(createdAt, ""),
		Policy:    policy,
		Versions: versiondata.Result{Node: versiondata.NodeData{
			LatestLTS:             "v" + targetVersion,
			LatestLTSFreshness:    versiondata.FreshnessFresh,
			AvailableVersions:     []string{"v22.0.0", "v" + targetVersion},
			ReleaseIndexFreshness: versiondata.FreshnessFresh,
		}},
		CreatedAt: createdAt,
		TTL:       plan.DefaultTTL,
	}, digestID('a'), digestID('b'))
	if err != nil {
		t.Fatalf("BuildExecutable() error = %v", err)
	}
	return value
}

func workflowDefaultPlan(
	t *testing.T,
	createdAt time.Time,
	targetVersion string,
) plan.Plan {
	t.Helper()
	value, err := plan.BuildDefaultSet(plan.DefaultSetInput{
		Inventory:             workflowInventory(createdAt, targetVersion),
		CreatedAt:             createdAt,
		TargetVersion:         targetVersion,
		ScriptDigest:          digestID('a'),
		CurrentAliasDigest:    aliasDigest("22"),
		CurrentAlias:          "22",
		CurrentDefaultVersion: "v22.0.0",
	})
	if err != nil {
		t.Fatalf("BuildDefaultSet() error = %v", err)
	}
	return value
}

func workflowNodeToolsPlan(
	t *testing.T,
	createdAt time.Time,
	nodeVersion string,
	targets NodeToolTargets,
) plan.Plan {
	t.Helper()
	selected := make([]plan.NodeToolTarget, 0, 3)
	if targets.NPM != "" {
		selected = append(selected, plan.NodeToolTarget{
			ToolID: plan.NodeToolNPM, CurrentVersion: "11.6.2",
			TargetVersion: targets.NPM, Provider: plan.NodeToolProviderNPM,
			ControlDigest: digestID('c'),
		})
	}
	if targets.Corepack != "" {
		selected = append(selected, plan.NodeToolTarget{
			ToolID: plan.NodeToolCorepack, CurrentVersion: "0.34.5",
			TargetVersion: targets.Corepack, Provider: plan.NodeToolProviderNPM,
			ControlDigest: digestID('d'),
		})
	}
	if targets.PNPM != "" {
		selected = append(selected, plan.NodeToolTarget{
			ToolID: plan.NodeToolPNPM, CurrentVersion: "10.0.0",
			TargetVersion: targets.PNPM, Provider: plan.NodeToolProviderCorepack,
			ControlDigest: digestID('e'),
		})
	}
	value, err := plan.BuildNodeTools(plan.NodeToolsInput{
		Inventory:          workflowInventory(createdAt, nodeVersion),
		CreatedAt:          createdAt,
		NodeVersion:        nodeVersion,
		NVMScriptDigest:    digestID('a'),
		DefaultAliasDigest: digestID('b'),
		Targets:            selected,
	})
	if err != nil {
		t.Fatalf("BuildNodeTools() error = %v", err)
	}
	return value
}

func workflowInventory(
	generatedAt time.Time,
	targetVersion string,
) inventory.Inventory {
	installations := []inventory.Installation{{
		ID: "node-nvm-22", Version: "v22.0.0",
		Path: "$HOME/.nvm/versions/node/v22.0.0/bin/node", Manager: "nvm",
		Architecture: inventory.ArchitectureARM64,
		ActiveState:  inventory.ActiveStateActive,
		DefaultState: inventory.DefaultStateDefault,
	}}
	if targetVersion != "" && targetVersion != "22.0.0" {
		installations = append(installations, inventory.Installation{
			ID: "node-nvm-target", Version: "v" + targetVersion,
			Path:    "$HOME/.nvm/versions/node/v" + targetVersion + "/bin/node",
			Manager: "nvm", Architecture: inventory.ArchitectureARM64,
			ActiveState:  inventory.ActiveStateInactive,
			DefaultState: inventory.DefaultStateNonDefault,
		})
	}
	return inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion,
		GeneratedAt:   generatedAt,
		System: inventory.System{
			OS: inventory.OSMacOS, OSVersion: "15.0",
			Architecture: inventory.ArchitectureARM64,
		},
		Tools: []inventory.Tool{{
			ID: "runtime.node", DisplayName: "Node.js",
			Category:      inventory.CategoryRuntime,
			Installations: installations,
		}},
	}
}

func aliasDigest(value string) string {
	digest := sha256.Sum256([]byte(value + "\n"))
	return "sha256:" + hex.EncodeToString(digest[:])
}
