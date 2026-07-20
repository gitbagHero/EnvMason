package defaultversion

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

var defaultTestTime = time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)

func TestSetAndRestoreDefaultWithIndependentConfirmedPlans(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("I16 NVM execution is macOS-only and fixture requires bash")
	}
	service, directory, clock := defaultFixtureService(t)
	prepared, err := service.PrepareSet(t.Context(), SetOptions{ToolID: "runtime.node", Version: "24.14.0"})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Plan.SchemaVersion != plan.HighRiskExecutableSchemaVersion || prepared.Plan.Actions[0].Risk != plan.RiskR3 {
		t.Fatalf("set Plan = %#v", prepared.Plan)
	}
	if value := readDefaultAlias(t, directory); value != "22" {
		t.Fatalf("prepare changed alias to %q", value)
	}
	if _, err := os.Stat(service.HistoryRoot); !os.IsNotExist(err) {
		t.Fatalf("prepare created history: %v", err)
	}

	*clock = defaultTestTime.Add(time.Minute)
	setResult, err := service.Execute(t.Context(), prepared, receipt(prepared.Plan, defaultTestTime.Add(30*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if setResult.Record.State != execution.StateCompleted || readDefaultAlias(t, directory) != "v24.14.0" {
		t.Fatalf("set result = %#v, alias=%q", setResult, readDefaultAlias(t, directory))
	}
	step := setResult.Record.Steps[0]
	if step.Before == nil || step.After == nil || step.Before.Facts["default_alias"] != "22" || step.After.Facts["default_alias"] != "v24.14.0" {
		t.Fatalf("set snapshots = %#v", step)
	}

	*clock = defaultTestTime.Add(2 * time.Minute)
	restore, err := service.PrepareRestore(t.Context(), RestoreOptions{OperationID: setResult.Record.ID})
	if err != nil {
		t.Fatal(err)
	}
	if restore.Plan.ID == prepared.Plan.ID || restore.Plan.Actions[0].Operation != "restore_default" || restore.Plan.Actions[0].Recovery.Mode != "manual" {
		t.Fatalf("restore Plan = %#v", restore.Plan)
	}
	*clock = defaultTestTime.Add(3 * time.Minute)
	restoreResult, err := service.Execute(t.Context(), restore, receipt(restore.Plan, defaultTestTime.Add(150*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if restoreResult.Record.State != execution.StateCompleted || readDefaultAlias(t, directory) != "22" {
		t.Fatalf("restore result = %#v, alias=%q", restoreResult, readDefaultAlias(t, directory))
	}
}

func TestDefaultSetRejectsMissingTargetConfirmationAndDriftWithoutWrites(t *testing.T) {
	service, directory, clock := defaultFixtureService(t)
	missing := service
	missing.Scan = func(context.Context) (inventory.Inventory, error) { return defaultInventory(directory, false), nil }
	if _, err := missing.PrepareSet(t.Context(), SetOptions{ToolID: "runtime.node", Version: "24.14.0"}); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Fatalf("missing target error = %v", err)
	}

	prepared, err := service.PrepareSet(t.Context(), SetOptions{ToolID: "runtime.node", Version: "24.14.0"})
	if err != nil {
		t.Fatal(err)
	}
	*clock = defaultTestTime.Add(time.Minute)
	wrong := execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: "sha256:" + strings.Repeat("0", 64), ConfirmedAt: defaultTestTime.Add(30 * time.Second)}
	if _, err := service.Execute(t.Context(), prepared, wrong); err == nil {
		t.Fatal("wrong confirmation was accepted")
	}
	if readDefaultAlias(t, directory) != "22" {
		t.Fatal("wrong confirmation changed alias")
	}

	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("v24.14.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Execute(t.Context(), prepared, receipt(prepared.Plan, defaultTestTime.Add(45*time.Second))); err == nil || !strings.Contains(err.Error(), "changed after review") {
		t.Fatalf("drift error = %v", err)
	}
	if _, err := os.Stat(service.HistoryRoot); !os.IsNotExist(err) {
		t.Fatalf("rejected operations created history: %v", err)
	}
}

func TestVerificationFailureRecordsRecoveryAndExternalChangeBlocksIt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("I16 NVM execution is macOS-only and fixture requires bash")
	}
	service, directory, clock := defaultFixtureService(t)
	prepared, err := service.PrepareSet(t.Context(), SetOptions{ToolID: "runtime.node", Version: "24.14.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "fail-new-shell"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	*clock = defaultTestTime.Add(time.Minute)
	result, err := service.Execute(t.Context(), prepared, receipt(prepared.Plan, defaultTestTime.Add(30*time.Second)))
	var failure *execution.ExecutionError
	if !errors.As(err, &failure) || failure.Code != execution.CodeVerificationFailed || result.Record.State != execution.StateFailed {
		t.Fatalf("verification failure = %#v, %v", result, err)
	}
	if result.Record.Steps[0].After == nil || readDefaultAlias(t, directory) != "v24.14.0" {
		t.Fatalf("failed operation lacks recoverable state: %#v", result.Record)
	}
	if err := os.Remove(filepath.Join(directory, "fail-new-shell")); err != nil {
		t.Fatal(err)
	}
	*clock = defaultTestTime.Add(2 * time.Minute)
	restore, err := service.PrepareRestore(t.Context(), RestoreOptions{OperationID: result.Record.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("v22.0.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	*clock = defaultTestTime.Add(3 * time.Minute)
	if _, err := service.Execute(t.Context(), restore, receipt(restore.Plan, defaultTestTime.Add(150*time.Second))); err == nil || !strings.Contains(err.Error(), "changed after review") {
		t.Fatalf("external change restore error = %v", err)
	}
}

func TestSecondFreshSetPlanSkipsSatisfiedAliasAndStillVerifies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("I16 NVM execution is macOS-only and fixture requires bash")
	}
	service, _, clock := defaultFixtureService(t)
	first, err := service.PrepareSet(t.Context(), SetOptions{ToolID: "runtime.node", Version: "24.14.0"})
	if err != nil {
		t.Fatal(err)
	}
	*clock = defaultTestTime.Add(time.Minute)
	if _, err := service.Execute(t.Context(), first, receipt(first.Plan, defaultTestTime.Add(30*time.Second))); err != nil {
		t.Fatal(err)
	}
	*clock = defaultTestTime.Add(2 * time.Minute)
	second, err := service.PrepareSet(t.Context(), SetOptions{ToolID: "runtime.node", Version: "24.14.0"})
	if err != nil {
		t.Fatal(err)
	}
	*clock = defaultTestTime.Add(3 * time.Minute)
	result, err := service.Execute(t.Context(), second, receipt(second.Plan, defaultTestTime.Add(150*time.Second)))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Record.Steps[0].Skipped || result.Record.Steps[0].Verification.State != execution.CheckPassed {
		t.Fatalf("idempotent result = %#v", result)
	}
}

func defaultFixtureService(t *testing.T) (Service, string, *time.Time) {
	t.Helper()
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "alias"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	createNode(t, directory, "22.0.0")
	createNode(t, directory, "24.14.0")
	nvmScript := `nvm() {
  command=$1
  shift
  if [ "$command" = alias ] && [ "$1" = default ]; then
    printf '%s\n' "$2" > "$NVM_DIR/alias/default"
    return 0
  fi
  return 64
}
if [ "${1-}" != --no-use ] && [ ! -f "$NVM_DIR/fail-new-shell" ]; then
  alias_value=$(sed -n '1p' "$NVM_DIR/alias/default")
  case "$alias_value" in
    v*.*.*) selected=$alias_value ;;
    *) selected=$(find "$NVM_DIR/versions/node" -mindepth 1 -maxdepth 1 -type d -name "v${alias_value}.*" | sort | tail -n 1 | sed 's|.*/||') ;;
  esac
  PATH="$NVM_DIR/versions/node/$selected/bin:$PATH"
  export PATH
fi
`
	if err := os.WriteFile(filepath.Join(directory, "nvm.sh"), []byte(nvmScript), 0o644); err != nil {
		t.Fatal(err)
	}
	clock := defaultTestTime
	home := t.TempDir()
	environment := map[string]string{"HOME": home, "NVM_DIR": directory, "TMPDIR": t.TempDir()}
	service := Service{
		GOOS: "darwin", Now: func() time.Time { return clock },
		LookupEnv: func(key string) (string, bool) { value, ok := environment[key]; return value, ok },
		Scan:      func(context.Context) (inventory.Inventory, error) { return defaultInventory(directory, true), nil },
		Runner:    execution.OSRunner{}, HistoryRoot: filepath.Join(t.TempDir(), "operations"),
	}
	return service, directory, &clock
}

func defaultInventory(directory string, includeTarget bool) inventory.Inventory {
	aliasData, _ := os.ReadFile(filepath.Join(directory, "alias", "default"))
	alias := strings.TrimSpace(string(aliasData))
	defaultVersion := "22.0.0"
	if strings.Contains(alias, "24") {
		defaultVersion = "24.14.0"
	}
	installations := []inventory.Installation{defaultInstallation(directory, "22.0.0", true, defaultVersion == "22.0.0")}
	if includeTarget {
		installations = append(installations, defaultInstallation(directory, "24.14.0", false, defaultVersion == "24.14.0"))
	}
	return inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion, GeneratedAt: defaultTestTime,
		System:   inventory.System{OS: inventory.OSMacOS, OSVersion: "15.0", Architecture: inventory.ArchitectureARM64},
		Tools:    []inventory.Tool{{ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime, Installations: installations}},
		Findings: []inventory.Finding{},
	}
}

func defaultInstallation(directory, version string, active, defaultState bool) inventory.Installation {
	activeState := inventory.ActiveStateInactive
	if active {
		activeState = inventory.ActiveStateActive
	}
	defaultValue := inventory.DefaultStateNonDefault
	if defaultState {
		defaultValue = inventory.DefaultStateDefault
	}
	return inventory.Installation{
		ID: "node-nvm-" + strings.ReplaceAll(version, ".", "-"), Version: "v" + version, NormalizedVersion: version,
		Path: filepath.Join(directory, "versions", "node", "v"+version, "bin", "node"), Manager: "nvm",
		Architecture: inventory.ArchitectureARM64, ActiveState: activeState, DefaultState: defaultValue,
		InstallReason: inventory.InstallReasonDirect, Sources: []inventory.SourceMetadata{},
	}
}

func createNode(t *testing.T, directory, version string) {
	t.Helper()
	bin := filepath.Join(directory, "versions", "node", "v"+version, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\nprintf 'v"+version+"\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func readDefaultAlias(t *testing.T, directory string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(directory, "alias", "default"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func receipt(value plan.Plan, confirmedAt time.Time) execution.ConfirmationReceipt {
	return execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: value.ID, ConfirmedAt: confirmedAt}
}
