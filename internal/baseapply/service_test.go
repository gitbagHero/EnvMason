package baseapply

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/adapter/homebrewinstall"
	"github.com/gitbagHero/EnvMason/internal/baseinstall"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/lockfile"
	"github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/profile"
)

var applyTestTime = time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)

func TestServiceCompletesFixedReviewedInstallAndRevalidatesRecovery(t *testing.T) {
	requireMacOSExecutorFixture(t)
	fixture := newApplyFixture(t)
	world := &applyWorld{}
	store := &applyStore{}
	record, err := fixture.service(world, store).Execute(
		t.Context(), fixture.prepared, fixture.snapshots, fixture.confirmation,
	)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if record.State != execution.StateCompleted || len(record.Steps) != 1 ||
		record.Steps[0].State != execution.StateCompleted ||
		record.Steps[0].Verification.State != execution.CheckPassed ||
		record.ConfirmedPlan == nil || record.ConfirmedPlan.ID != fixture.prepared.Plan.ID ||
		world.writeCalls != 1 || len(world.writeSpecs) != 1 {
		t.Fatalf("record/world = %#v / %#v", record, world)
	}
	step := record.Steps[0]
	if step.Invocation == nil || step.Invocation.Executable != "/opt/homebrew/bin/brew" ||
		!reflect.DeepEqual(step.Invocation.Args, []string{"install", "--formula", "--force-bottle", "git"}) ||
		step.Before == nil || step.Before.Facts["formula.git"] != "absent" ||
		step.After == nil || step.After.Facts["formula.git"] != "2.51.0" ||
		step.After.Facts["review_id"] != fixture.prepared.Review.ID ||
		step.After.Facts["plan_id"] != fixture.prepared.Plan.ID {
		t.Fatalf("step = %#v", step)
	}
	assertFixedWriteSpec(t, world.writeSpecs[0])
	if strings.Contains(string(store.data[len(store.data)-1]), "/Users/envmason") ||
		strings.Contains(string(store.data[len(store.data)-1]), "/private/tmp/envmason") {
		t.Fatal("operation history leaked controlled private paths")
	}

	registry := fixture.registry(t, world)
	revalidated, err := execution.RevalidateRecovery(t.Context(), record, registry)
	if err != nil || len(revalidated.Candidates) != 1 ||
		revalidated.Candidates[0].CurrentState != execution.RecoveryCheckpointCurrent {
		t.Fatalf("current recovery checkpoint = %#v, %v", revalidated, err)
	}
	world.installed = false
	revalidated, err = execution.RevalidateRecovery(t.Context(), record, registry)
	if err != nil || revalidated.Candidates[0].CurrentState != execution.RecoveryCheckpointDrifted {
		t.Fatalf("drifted recovery checkpoint = %#v, %v", revalidated, err)
	}
}

func TestServiceRejectsUnconfirmedUnsupportedOrDriftedExecutionBeforeWrite(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*applyFixture, *Service)
	}{
		{"wrong final Plan confirmation", func(value *applyFixture, _ *Service) {
			value.confirmation.ConfirmedPlanID = value.prepared.CandidatePlan.ID
		}},
		{"wrong Review confirmation", func(value *applyFixture, _ *Service) {
			value.confirmation.ConfirmedReviewID = applyDigest('f')
		}},
		{"unsupported operating system", func(_ *applyFixture, service *Service) { service.GOOS = "linux" }},
		{"controlled HOME drift", func(value *applyFixture, _ *Service) {
			value.snapshots.Configuration.Environment["HOME"] = "/Users/changed"
		}},
		{"controlled TMPDIR drift", func(value *applyFixture, _ *Service) {
			value.snapshots.Configuration.Environment["TMPDIR"] = "/private/tmp/changed"
		}},
		{"executable bytes drift", func(value *applyFixture, _ *Service) {
			value.snapshots.ExecutableData = []byte("changed executable")
		}},
		{"catalog bytes drift", func(value *applyFixture, _ *Service) {
			value.snapshots.CatalogJSON = append(value.snapshots.CatalogJSON, '\n')
		}},
		{"artifact size drift", func(value *applyFixture, _ *Service) {
			value.snapshots.Artifacts[0].DownloadBytes++
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newApplyFixture(t)
			world := &applyWorld{}
			store := &applyStore{}
			service := fixture.service(world, store)
			test.mutate(&fixture, &service)
			record, err := service.Execute(t.Context(), fixture.prepared, fixture.snapshots, fixture.confirmation)
			if err == nil || record.State == execution.StateCompleted || world.writeCalls != 0 || len(store.records) != 0 {
				t.Fatalf("record/error/world/store = %#v / %v / %#v / %#v", record, err, world, store)
			}
		})
	}
}

func TestServicePersistsPreflightAndProcessFailuresWithoutFalseCompletion(t *testing.T) {
	requireMacOSExecutorFixture(t)
	t.Run("conflicting installed version", func(t *testing.T) {
		fixture := newApplyFixture(t)
		world := &applyWorld{probeMode: "conflict"}
		store := &applyStore{}
		record, err := fixture.service(world, store).Execute(
			t.Context(), fixture.prepared, fixture.snapshots, fixture.confirmation,
		)
		assertExecutionFailure(t, err, execution.CodePreconditionFailed)
		if record.State != execution.StateFailed || record.Steps[0].Precondition.State != execution.CheckFailed ||
			world.writeCalls != 0 || len(store.records) == 0 || store.records[len(store.records)-1].State == execution.StateCompleted {
			t.Fatalf("record/world/store = %#v / %#v / %#v", record, world, store)
		}
	})

	t.Run("write process failure", func(t *testing.T) {
		fixture := newApplyFixture(t)
		world := &applyWorld{writeFailure: true}
		store := &applyStore{}
		record, err := fixture.service(world, store).Execute(
			t.Context(), fixture.prepared, fixture.snapshots, fixture.confirmation,
		)
		assertExecutionFailure(t, err, execution.CodeExitNonZero)
		if record.State != execution.StateFailed || record.Steps[0].State != execution.StateFailed ||
			record.Steps[0].After == nil || world.writeCalls != 1 ||
			store.records[len(store.records)-1].State == execution.StateCompleted {
			t.Fatalf("record/world = %#v / %#v", record, world)
		}
		assessment, assessErr := execution.AssessRecovery(record)
		if assessErr != nil || len(assessment.Candidates) != 1 ||
			assessment.Candidates[0].Evidence != execution.RecoveryEvidenceUncertain ||
			assessment.Candidates[0].RecoveryMode != "manual" {
			t.Fatalf("recovery assessment = %#v, %v", assessment, assessErr)
		}
	})

	t.Run("post-install verification failure", func(t *testing.T) {
		fixture := newApplyFixture(t)
		world := &applyWorld{writeDoesNotInstall: true}
		store := &applyStore{}
		record, err := fixture.service(world, store).Execute(
			t.Context(), fixture.prepared, fixture.snapshots, fixture.confirmation,
		)
		assertExecutionFailure(t, err, execution.CodeVerificationFailed)
		if record.State != execution.StateFailed || record.Steps[0].Verification.State != execution.CheckFailed ||
			world.writeCalls != 1 || store.records[len(store.records)-1].State == execution.StateCompleted {
			t.Fatalf("record/world = %#v / %#v", record, world)
		}
	})
}

func TestServicePreservesCompletedAndFailedStatesAcrossTwoFormulae(t *testing.T) {
	requireMacOSExecutorFixture(t)
	for _, test := range []struct {
		name           string
		failFormula    string
		wantStates     []execution.State
		wantWriteCalls int
	}{
		{"first action fails", "cmake", []execution.State{execution.StateFailed, execution.StatePending}, 1},
		{"second action fails", "git", []execution.State{execution.StateCompleted, execution.StateFailed}, 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newApplyFixtureWithCMake(t)
			world := &applyWorld{writeFailFormula: test.failFormula}
			store := &applyStore{}
			record, err := fixture.service(world, store).Execute(
				t.Context(), fixture.prepared, fixture.snapshots, fixture.confirmation,
			)
			assertExecutionFailure(t, err, execution.CodeExitNonZero)
			if record.State != execution.StateFailed || len(record.Steps) != 2 ||
				record.Steps[0].ActionID != "install-base-cmake" || record.Steps[0].State != test.wantStates[0] ||
				record.Steps[1].ActionID != "install-base-git" || record.Steps[1].State != test.wantStates[1] ||
				world.writeCalls != test.wantWriteCalls || len(world.writeSpecs) != test.wantWriteCalls ||
				world.writeSpecs[0].Args[3] != "cmake" ||
				store.records[len(store.records)-1].State != execution.StateFailed {
				t.Fatalf("record/world = %#v / %#v", record, world)
			}
			if test.wantWriteCalls == 2 && world.writeSpecs[1].Args[3] != "git" {
				t.Fatalf("write specs = %#v", world.writeSpecs)
			}
		})
	}
}

func TestServiceSkipsWriteWhenReviewedStateBecomesExactDuringPreflight(t *testing.T) {
	requireMacOSExecutorFixture(t)
	fixture := newApplyFixture(t)
	world := &applyWorld{installOnProbe: 3}
	store := &applyStore{}
	record, err := fixture.service(world, store).Execute(
		t.Context(), fixture.prepared, fixture.snapshots, fixture.confirmation,
	)
	if err != nil || record.State != execution.StateCompleted || !record.Steps[0].Skipped ||
		world.writeCalls != 0 || record.Steps[0].Verification.State != execution.CheckPassed {
		t.Fatalf("record/error/world = %#v / %v / %#v", record, err, world)
	}
}

func TestInstallDefinitionsRejectMismatchedCandidateFinalPlanBinding(t *testing.T) {
	fixture := newApplyFixture(t)
	world := &applyWorld{}
	_, err := homebrewinstall.InstallDefinitions(homebrewinstall.InstallOptions{
		CandidatePlan:  fixture.prepared.Plan,
		Plan:           fixture.prepared.Plan,
		Review:         fixture.prepared.Review,
		BrewPath:       fixture.snapshots.ExecutablePath,
		ExecutableData: fixture.snapshots.ExecutableData,
		Configuration:  fixture.snapshots.Configuration,
		Verifier:       applyProbeRunner{world: world},
	})
	if err == nil {
		t.Fatal("review-bound Plan was accepted as its own unreviewed candidate")
	}
}

func TestInstallDefinitionsFreezeReviewedInputs(t *testing.T) {
	fixture := newApplyFixture(t)
	world := &applyWorld{}
	options := homebrewinstall.InstallOptions{
		CandidatePlan: fixture.prepared.CandidatePlan, Plan: fixture.prepared.Plan,
		Review: fixture.prepared.Review, BrewPath: fixture.snapshots.ExecutablePath,
		ExecutableData: fixture.snapshots.ExecutableData, Configuration: fixture.snapshots.Configuration,
		Verifier: applyProbeRunner{world: world},
	}
	action := options.Plan.Actions[0]
	definitions, err := homebrewinstall.InstallDefinitions(options)
	if err != nil || len(definitions) != 1 {
		t.Fatalf("InstallDefinitions() = %d, %v", len(definitions), err)
	}
	options.CandidatePlan.Actions[0].TargetVersion = "changed"
	options.Plan.Actions[0].TargetVersion = "changed"
	options.Review.Actions[0].Root.Version = "changed"
	options.ExecutableData[0] = 'x'
	options.Configuration.Environment["HOME"] = "/Users/changed"
	spec, err := definitions[0].Build(action)
	if err != nil {
		t.Fatalf("cloned definition Build() error = %v", err)
	}
	assertFixedWriteSpec(t, spec)
	action.TargetVersion = "changed"
	if _, err := definitions[0].Build(action); err == nil {
		t.Fatal("definition accepted a mutated Plan action")
	}
}

type applyFixture struct {
	now          time.Time
	prepared     Prepared
	snapshots    FreshSnapshots
	confirmation Confirmation
}

func newApplyFixture(t *testing.T) applyFixture {
	return buildApplyFixture(t, false)
}

func newApplyFixtureWithCMake(t *testing.T) applyFixture {
	return buildApplyFixture(t, true)
}

func buildApplyFixture(t *testing.T, installCMake bool) applyFixture {
	t.Helper()
	catalog := []map[string]any{
		applyCatalogFormula("git", "2.51.0", []string{"gettext"}, 'a'),
		applyCatalogFormula("gettext", "0.26", []string{}, 'b'),
	}
	if installCMake {
		catalog = append(catalog, applyCatalogFormula("cmake", "4.0.3", []string{"gettext"}, 'c'))
	}
	catalogJSON, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := applyTestTime.Add(-5 * time.Minute)
	source := lockfile.Source{
		ID: "homebrew-core", Kind: lockfile.SourcePackageCatalog,
		URI:        "https://formulae.brew.sh/api/formula.json",
		SnapshotAt: createdAt.Add(-2 * time.Hour), Digest: applyBytesDigest(catalogJSON),
	}
	conditions := lockfile.Conditions{OS: inventory.OSMacOS, Architectures: []inventory.Architecture{inventory.ArchitectureARM64}}
	item := func(capability, formula, version string, state lockfile.ResolutionState) lockfile.Item {
		value := lockfile.Item{
			ID: capability, Module: profile.ModuleBase, Capability: capability, State: state,
			Implementation: &lockfile.Implementation{
				ToolID: "homebrew.formula." + formula, Manager: "homebrew", PackageKind: "formula",
				PackageID: formula, Version: version, SourceID: source.ID, Conditions: conditions,
			},
			Observed: []lockfile.Observation{}, Reason: lockfile.ReasonInstallationRequired,
		}
		if state == lockfile.StateSatisfied {
			value.Observed = []lockfile.Observation{{Manager: "homebrew", Version: version}}
			value.Reason = lockfile.ReasonCompatibleInstallation
		}
		return value
	}
	cmakeState := lockfile.StateSatisfied
	if installCMake {
		cmakeState = lockfile.StateInstallRequired
	}
	lockValue, err := lockfile.Build(lockfile.BuildInput{
		GeneratedAt: createdAt.Add(-time.Minute),
		Profile:     profile.Profile{SchemaVersion: profile.SchemaVersion, Name: "base", Modules: []profile.Module{{ID: profile.ModuleBase}}},
		Target:      lockfile.Target{OS: inventory.OSMacOS, OSVersion: "15.6", Architecture: inventory.ArchitectureARM64},
		Sources:     []lockfile.Source{source},
		Items: []lockfile.Item{
			item("base.git", "git", "2.51.0", lockfile.StateInstallRequired),
			item("base.cmake", "cmake", "4.0.3", cmakeState),
		},
	})
	if err != nil {
		t.Fatalf("build fixture Lock: %v", err)
	}
	candidate, err := plan.BuildBaseInstall(plan.BaseInstallInput{
		Lock: lockValue, CreatedAt: createdAt,
		Inventory: applyInventory(createdAt.Add(-time.Minute), false),
	})
	if err != nil {
		t.Fatalf("build fixture Plan: %v", err)
	}
	configuration := baseinstall.ConfigurationSnapshot{Environment: map[string]string{
		"HOME": "/Users/envmason", "TMPDIR": "/private/tmp/envmason",
		"HOMEBREW_NO_ANALYTICS": "1", "HOMEBREW_NO_ASK": "1", "HOMEBREW_NO_AUTO_UPDATE": "1",
		"HOMEBREW_NO_ENV_HINTS": "1", "HOMEBREW_NO_INSTALL_CLEANUP": "1",
		"HOMEBREW_NO_INSTALL_UPGRADE": "1", "HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK": "1",
	}}
	reviewAt := applyTestTime.Add(-time.Minute)
	reviewInventory := applyInventory(reviewAt, true)
	artifacts := []baseinstall.BottleArtifact{{
		Formula: "git", Version: "2.51.0", BottleTag: "arm64_sequoia",
		SHA256: applyDigest('a'), DownloadBytes: 10,
	}}
	if installCMake {
		artifacts = append(artifacts, baseinstall.BottleArtifact{
			Formula: "cmake", Version: "4.0.3", BottleTag: "arm64_sequoia",
			SHA256: applyDigest('c'), DownloadBytes: 20,
		})
	}
	facts, err := baseinstall.CollectTransactionFacts(baseinstall.TransactionCollectionInput{
		ObservedAt: reviewAt, Plan: candidate, Lock: lockValue, Inventory: reviewInventory,
		ExecutablePath: "/opt/homebrew/bin/brew", ExecutableData: []byte("fixed brew executable"),
		Configuration: configuration, CatalogJSON: catalogJSON, BottleTag: "arm64_sequoia", Artifacts: artifacts,
	})
	if err != nil {
		t.Fatalf("collect fixture review: %v", err)
	}
	review, err := baseinstall.PrepareTransactionReview(baseinstall.TransactionReviewInput{
		PreparedAt: reviewAt, Plan: candidate, Lock: lockValue, Baseline: facts.Baseline, Actions: facts.Actions,
	})
	if err != nil {
		t.Fatalf("prepare fixture review: %v", err)
	}
	prepared, err := Prepare(candidate, lockValue, review)
	if err != nil {
		t.Fatalf("prepare fixture execution: %v", err)
	}
	return applyFixture{
		now: applyTestTime, prepared: prepared,
		snapshots: FreshSnapshots{
			Inventory: applyInventory(applyTestTime, true), ExecutablePath: "/opt/homebrew/bin/brew",
			ExecutableData: []byte("fixed brew executable"), Configuration: configuration,
			CatalogJSON: catalogJSON, BottleTag: "arm64_sequoia", Artifacts: artifacts,
		},
		confirmation: Confirmation{
			Scope: ConfirmationScope, ConfirmedPlanID: prepared.Plan.ID,
			ConfirmedReviewID: review.ID, ConfirmedAt: reviewAt.Add(30 * time.Second),
		},
	}
}

func (fixture applyFixture) service(world *applyWorld, store *applyStore) Service {
	return Service{
		GOOS: "darwin", Now: func() time.Time { return fixture.now },
		Runner: applyWriteRunner{world: world}, Verifier: applyProbeRunner{world: world}, Store: store,
		NewOperationID: func() (string, error) { return "op-00000000000000000000000000000001", nil },
	}
}

func (fixture applyFixture) registry(t *testing.T, world *applyWorld) execution.Registry {
	t.Helper()
	definitions, err := homebrewinstall.InstallDefinitions(homebrewinstall.InstallOptions{
		CandidatePlan: fixture.prepared.CandidatePlan, Plan: fixture.prepared.Plan,
		Review: fixture.prepared.Review, BrewPath: fixture.snapshots.ExecutablePath,
		ExecutableData: fixture.snapshots.ExecutableData, Configuration: fixture.snapshots.Configuration,
		Verifier: applyProbeRunner{world: world},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := execution.NewRegistry(definitions...)
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

type applyWorld struct {
	installed           bool
	probeCalls          int
	installOnProbe      int
	probeMode           string
	writeFailure        bool
	writeFailFormula    string
	writeDoesNotInstall bool
	writeCalls          int
	writeSpecs          []execution.CommandSpec
	installedFormulae   map[string]bool
}

type applyProbeRunner struct{ world *applyWorld }

func (runner applyProbeRunner) Run(_ context.Context, spec execution.CommandSpec) execution.ProcessResult {
	code := 0
	versions := reflect.DeepEqual(spec.Args, []string{"list", "--formula", "--versions"})
	fullNames := reflect.DeepEqual(spec.Args, []string{"list", "--formula", "--full-name"})
	if !versions && !fullNames {
		code = 1
		return execution.ProcessResult{ExitCode: &code}
	}
	if versions {
		runner.world.probeCalls++
		if runner.world.installOnProbe == runner.world.probeCalls {
			runner.world.installed = true
		}
		if runner.world.probeMode == "malformed" {
			return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: "{"}}
		}
	}
	lines := []string{"gettext"}
	versionByFormula := map[string]string{"gettext": "0.26"}
	if runner.world.installed || runner.world.probeMode == "conflict" {
		version := "2.51.0"
		if runner.world.probeMode == "conflict" {
			version = "2.50.0"
		}
		lines = append(lines, "git")
		versionByFormula["git"] = version
	}
	if runner.world.installedFormulae["cmake"] {
		lines = append(lines, "cmake")
		versionByFormula["cmake"] = "4.0.3"
	}
	sort.Strings(lines)
	if fullNames {
		return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: strings.Join(lines, "\n") + "\n"}}
	}
	for index, formula := range lines {
		lines[index] = formula + " " + versionByFormula[formula]
	}
	return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: strings.Join(lines, "\n") + "\n"}}
}

type applyWriteRunner struct{ world *applyWorld }

func (runner applyWriteRunner) Run(_ context.Context, spec execution.CommandSpec) execution.ProcessResult {
	runner.world.writeCalls++
	runner.world.writeSpecs = append(runner.world.writeSpecs, spec)
	formula := spec.Args[3]
	if runner.world.writeFailure || runner.world.writeFailFormula == formula {
		code := 7
		return execution.ProcessResult{ExitCode: &code, Failure: &execution.ExecutionError{
			Code: execution.CodeExitNonZero, Message: "injected fixed process failure",
		}}
	}
	if runner.world.writeDoesNotInstall {
		code := 0
		return execution.ProcessResult{ExitCode: &code}
	}
	if runner.world.installedFormulae == nil {
		runner.world.installedFormulae = make(map[string]bool)
	}
	runner.world.installedFormulae[formula] = true
	if formula == "git" {
		runner.world.installed = true
	}
	code := 0
	return execution.ProcessResult{ExitCode: &code}
}

type applyStore struct {
	records []execution.Record
	data    [][]byte
}

func (store *applyStore) Save(record execution.Record) error {
	data, err := execution.MarshalRecord(record)
	if err != nil {
		return err
	}
	cloned, err := execution.DecodeRecord(data)
	if err != nil {
		return err
	}
	store.data = append(store.data, data)
	store.records = append(store.records, cloned)
	return nil
}

func applyInventory(at time.Time, includeGettext bool) inventory.Inventory {
	source := inventory.SourceMetadata{Kind: inventory.SourceFixture, Name: "apply fixture", CollectedAt: at, Confidence: inventory.ConfidenceHigh}
	tools := []inventory.Tool{{
		ID: "manager.homebrew", DisplayName: "Homebrew", Category: inventory.CategoryEcosystem,
		Installations: []inventory.Installation{{
			ID: "homebrew:manager", Version: "6.0.9", Path: "/opt/homebrew/bin/brew",
			Architecture: inventory.ArchitectureARM64, Manager: "homebrew",
			ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			InstallReason: inventory.InstallReasonDirect, Sources: []inventory.SourceMetadata{source},
		}},
	}}
	if includeGettext {
		tools = append(tools, inventory.Tool{
			ID: "homebrew.formula.gettext", DisplayName: "gettext", Category: inventory.CategoryUnknown,
			Installations: []inventory.Installation{{
				ID: "homebrew:formula:gettext:0.26", Version: "0.26", NormalizedVersion: "0.26",
				Path: "/opt/homebrew/Cellar/gettext/0.26", Architecture: inventory.ArchitectureARM64,
				Manager: "homebrew", ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
				InstallReason: inventory.InstallReasonDependency, Sources: []inventory.SourceMetadata{source},
			}},
		})
	}
	return inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion, GeneratedAt: at,
		System: inventory.System{
			OS: inventory.OSMacOS, OSVersion: "15.6", OSBuild: "24G90",
			Architecture: inventory.ArchitectureARM64, ProcessArchitecture: inventory.ArchitectureARM64,
			TranslationState: inventory.TranslationStateNative,
			Shell:            inventory.Shell{LoginPath: "/bin/zsh", LoginName: "zsh", InvokingPath: "/bin/zsh", InvokingName: "zsh"},
			PathEntries:      []inventory.PathEntry{}, Sources: []inventory.SourceMetadata{source},
		},
		Tools: tools, Findings: []inventory.Finding{},
	}
}

func applyCatalogFormula(name, version string, dependencies []string, digest byte) map[string]any {
	return map[string]any{
		"name": name, "full_name": name, "tap": "homebrew/core",
		"versions": map[string]any{"stable": version}, "revision": 0,
		"dependencies": dependencies, "recommended_dependencies": []string{},
		"bottle": map[string]any{"stable": map[string]any{"files": map[string]any{
			"arm64_sequoia": map[string]any{"sha256": strings.Repeat(string(digest), 64)},
		}}},
		"variations": map[string]any{}, "disabled": false,
	}
}

func applyBytesDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(digest[:])
}

func applyDigest(character byte) string {
	return "sha256:" + strings.Repeat(string(character), 64)
}

func assertFixedWriteSpec(t *testing.T, spec execution.CommandSpec) {
	t.Helper()
	wantEnvironment := []string{
		"HOME=/Users/envmason",
		"HOMEBREW_NO_ANALYTICS=1",
		"HOMEBREW_NO_ASK=1",
		"HOMEBREW_NO_AUTO_UPDATE=1",
		"HOMEBREW_NO_ENV_HINTS=1",
		"HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1",
		"HOMEBREW_NO_INSTALL_CLEANUP=1",
		"HOMEBREW_NO_INSTALL_UPGRADE=1",
		"PATH=/opt/homebrew/bin:/usr/bin:/bin:/usr/sbin:/sbin",
		"TMPDIR=/private/tmp/envmason",
	}
	if spec.Executable != "/opt/homebrew/bin/brew" ||
		!reflect.DeepEqual(spec.Args, []string{"install", "--formula", "--force-bottle", "git"}) ||
		!reflect.DeepEqual(spec.Environment, wantEnvironment) || spec.Directory != "/private/tmp/envmason" ||
		spec.Timeout != homebrewinstall.InstallTimeout || !spec.TerminateTree ||
		!reflect.DeepEqual(spec.SensitiveValues, []string{"/Users/envmason", "/private/tmp/envmason"}) {
		t.Fatalf("fixed write spec = %#v", spec)
	}
	for _, entry := range spec.Environment {
		if strings.Contains(strings.ToLower(entry), "proxy") {
			t.Fatalf("fixed write environment inherited proxy entry %q", entry)
		}
	}
}

func assertExecutionFailure(t *testing.T, err error, code execution.Code) {
	t.Helper()
	var failure *execution.ExecutionError
	if !errors.As(err, &failure) || failure.Code != code {
		t.Fatalf("error = %v, want %s", err, code)
	}
}

func requireMacOSExecutorFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("I21-D execution fixture uses reviewed absolute macOS paths")
	}
}
