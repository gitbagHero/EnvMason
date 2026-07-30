package nodetools

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	adapter "github.com/gitbagHero/EnvMason/internal/adapter/nodetools"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestValidateOptionsRequiresExactSelectedTargets(t *testing.T) {
	valid := Options{NodeVersion: "24.12.0", NPMVersion: "12.0.1"}
	if err := ValidateOptions(valid); err != nil {
		t.Fatal(err)
	}
	tests := []Options{
		{NodeVersion: "lts", NPMVersion: "12.0.1"},
		{NodeVersion: "24.12.0"},
		{NodeVersion: "24.12.0", NPMVersion: "latest"},
		{NodeVersion: "24.12.0", PNPMVersion: "11.1.0-beta.1"},
	}
	for _, value := range tests {
		if err := ValidateOptions(value); err == nil {
			t.Fatalf("ValidateOptions(%#v) succeeded", value)
		}
	}
}

func TestPrepareRejectsUnsupportedPlatformBeforeScan(t *testing.T) {
	scanCalled := false
	service := Service{
		GOOS: "windows",
		Scan: func(context.Context) (inventory.Inventory, error) {
			scanCalled = true
			return inventory.Inventory{}, nil
		},
	}
	_, err := service.Prepare(context.Background(), Options{
		NodeVersion: "24.12.0",
		NPMVersion:  "12.0.1",
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported on windows") {
		t.Fatalf("error = %v", err)
	}
	if scanCalled {
		t.Fatal("unsupported platform reached environment scan")
	}
}

func TestPrepareBuildsProviderBoundR2Plan(t *testing.T) {
	service, _, _ := nodeToolsServiceFixture(t)
	service.Runner = corepackVersionRunner("10.0.0")
	prepared, err := service.Prepare(context.Background(), Options{
		NodeVersion: "24.12.0", NPMVersion: "11.6.2", PNPMVersion: "11.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Plan.SchemaVersion != plan.ExecutableSchemaVersion || len(prepared.Plan.Actions) != 2 {
		t.Fatalf("Plan = %#v", prepared.Plan)
	}
	if prepared.Plan.Actions[0].Adapter != plan.NodeToolProviderNPM ||
		prepared.Plan.Actions[1].Adapter != plan.NodeToolProviderCorepack {
		t.Fatalf("actions = %#v", prepared.Plan.Actions)
	}
}

func TestExecuteUsesSharedExecutorAndSkipsSatisfiedTarget(t *testing.T) {
	service, _, activeNode := nodeToolsServiceFixture(t)
	prepared, err := service.Prepare(context.Background(), Options{NodeVersion: "24.12.0", NPMVersion: "11.6.2"})
	if err != nil {
		t.Fatal(err)
	}
	service.Runner = serviceRunner{versions: map[string]string{
		prepared.baseline.NPM.Executable: "11.6.2\n",
		prepared.baseline.NodeBinary:     "v24.12.0\n",
		activeNode:                       "v24.12.0\n",
	}}
	result, err := service.Execute(context.Background(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.State != execution.StateCompleted || len(result.Record.Steps) != 1 || !result.Record.Steps[0].Skipped {
		t.Fatalf("record = %#v", result.Record)
	}
	if result.RecordPath == "" {
		t.Fatal("operation record path is empty")
	}
	recordJSON, err := execution.MarshalRecord(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(recordJSON), prepared.baseline.NVM.Directory) ||
		!strings.Contains(string(recordJSON), "$NVM_DIR") {
		t.Fatalf("Node tools record leaked private Plan path: %s", recordJSON)
	}
}

func TestExecuteVerifiesSatisfiedCorepackProxyWithExactCurrentVersion(t *testing.T) {
	service, _, _ := nodeToolsServiceFixture(t)
	service.Runner = functionRunner(func(spec execution.CommandSpec) execution.ProcessResult {
		code := 0
		switch filepath.Base(spec.Executable) {
		case "pnpm":
			return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: "10.0.0\n"}}
		case "node":
			return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: "v24.12.0\n"}}
		default:
			return execution.ProcessResult{Failure: &execution.ExecutionError{Code: execution.CodeStartFailed, Message: "unexpected fixture command"}}
		}
	})
	prepared, err := service.Prepare(context.Background(), Options{NodeVersion: "24.12.0", PNPMVersion: "10.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Execute(context.Background(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	step := result.Record.Steps[0]
	if !step.Skipped || step.State != execution.StateCompleted || step.Before == nil || step.After == nil {
		t.Fatalf("step = %#v", step)
	}
	if step.Before.Facts["pnpm_package_version"] != "10.0.0" || step.Before.Facts["pnpm_path"] != "bin/pnpm" {
		t.Fatalf("before = %#v", step.Before)
	}
}

type serviceRunner struct {
	versions map[string]string
}

func (runner serviceRunner) Run(_ context.Context, spec execution.CommandSpec) execution.ProcessResult {
	code := 0
	value, ok := runner.versions[spec.Executable]
	if !ok {
		return execution.ProcessResult{Failure: &execution.ExecutionError{Code: execution.CodeStartFailed}}
	}
	return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: value}}
}

func nodeToolsServiceFixture(t *testing.T) (Service, string, string) {
	t.Helper()
	requirePOSIXFixture(t)
	home := t.TempDir()
	directory := filepath.Join(home, ".nvm")
	writeServiceFixture(t, filepath.Join(directory, "nvm.sh"), "fixture", 0o644)
	writeServiceFixture(t, filepath.Join(directory, "alias", "default"), "v24.12.0\n", 0o644)
	root := filepath.Join(directory, "versions", "node", "v24.12.0")
	node := filepath.Join(root, "bin", "node")
	writeServiceFixture(t, node, "node", 0o755)
	writeServicePackage(t, root, "npm", "11.6.2", "bin/npm-cli.js")
	writeServicePackage(t, root, "corepack", "0.34.5", "dist/corepack.js")
	writeServiceFixture(t, filepath.Join(root, "lib", "node_modules", "corepack", "dist", "pnpm.js"), "pnpm", 0o644)
	if err := os.Symlink("../lib/node_modules/corepack/dist/pnpm.js", filepath.Join(root, "bin", "pnpm")); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 28, 8, 0, 0, 0, time.UTC)
	value := inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion, GeneratedAt: now,
		System: inventory.System{OS: inventory.OSMacOS, OSVersion: "15.0", Architecture: inventory.ArchitectureARM64},
		Tools: []inventory.Tool{{
			ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime,
			Installations: []inventory.Installation{{
				ID: "node-nvm-target", Version: "v24.12.0", Path: node, Manager: "nvm",
				Architecture: inventory.ArchitectureARM64, ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault,
			}},
		}},
	}
	environment := map[string]string{"HOME": home, "NVM_DIR": directory, "TMPDIR": t.TempDir()}
	service := Service{
		GOOS: "darwin", Now: func() time.Time { return now },
		LookupEnv: func(key string) (string, bool) {
			result, ok := environment[key]
			return result, ok
		},
		Scan:   func(context.Context) (inventory.Inventory, error) { return value, nil },
		Runner: serviceRunner{versions: map[string]string{}}, HistoryRoot: t.TempDir(),
	}
	return service, filepath.Join(root, "bin", "npm"), node
}

func requirePOSIXFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Node ancillary-tool service fixture uses POSIX execute permissions and symlinks")
	}
}

func writeServicePackage(t *testing.T, root, name, version, entry string) {
	t.Helper()
	packageRoot := filepath.Join(root, "lib", "node_modules", name)
	writeServiceFixture(t, filepath.Join(packageRoot, "package.json"), `{"name":"`+name+`","version":"`+version+`"}`, 0o644)
	writeServiceFixture(t, filepath.Join(packageRoot, filepath.FromSlash(entry)), name, 0o644)
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join("..", "lib", "node_modules", name, filepath.FromSlash(entry))
	if err := os.Symlink(target, filepath.Join(root, "bin", name)); err != nil {
		t.Fatal(err)
	}
}

func writeServiceFixture(t *testing.T, path, value string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteRejectsDriftAfterPlanReview(t *testing.T) {
	service, _, _ := nodeToolsServiceFixture(t)
	prepared, err := service.Prepare(context.Background(), Options{NodeVersion: "24.12.0", NPMVersion: "12.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prepared.baseline.NPM.PackageRoot, "package.json"), []byte(`{"name":"npm","version":"11.6.3"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = service.Execute(context.Background(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err == nil || !strings.Contains(err.Error(), "changed after review") {
		t.Fatalf("error = %v", err)
	}
}

func TestExecuteRecordsVerifiedFailedAndPendingActionsSeparately(t *testing.T) {
	service, _, activeNode := nodeToolsServiceFixture(t)
	service.Runner = corepackVersionRunner("10.0.0")
	prepared, err := service.Prepare(context.Background(), Options{
		NodeVersion: "24.12.0", NPMVersion: "11.6.2",
		CorepackVersion: "0.35.0", PNPMVersion: "11.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	service.Runner = functionRunner(func(spec execution.CommandSpec) execution.ProcessResult {
		if len(spec.Args) == 1 && spec.Args[0] == "--version" {
			code := 0
			switch spec.Executable {
			case prepared.baseline.NPM.Executable:
				return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: "11.6.2\n"}}
			case prepared.baseline.PNPM.Executable:
				return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: "10.0.0\n"}}
			case prepared.baseline.NodeBinary, activeNode:
				return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: "v24.12.0\n"}}
			}
		}
		return execution.ProcessResult{Failure: &execution.ExecutionError{Code: execution.CodeExitNonZero, Message: "fixture failure"}}
	})
	result, err := service.Execute(context.Background(), prepared, execution.ConfirmationReceipt{
		Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: prepared.Plan.CreatedAt,
	})
	if err == nil || result.Record.State != execution.StateFailed || len(result.Record.Steps) != 3 {
		t.Fatalf("result = %#v, %v", result, err)
	}
	if result.Record.Steps[0].State != execution.StateCompleted ||
		result.Record.Steps[1].State != execution.StateFailed ||
		result.Record.Steps[2].State != execution.StatePending {
		t.Fatalf("step states = %#v", result.Record.Steps)
	}
	currentInventory, err := service.Scan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	_, checkpointOptions, err := service.inspect(currentInventory, prepared.baseline.NodeVersion)
	if err != nil {
		t.Fatal(err)
	}
	checkpointOptions.Targets = prepared.targets
	registry, err := execution.NewRegistry(adapter.Definitions(checkpointOptions)...)
	if err != nil {
		t.Fatal(err)
	}
	assessment, err := execution.AssessContinuation(t.Context(), result.Record, registry)
	if err != nil {
		t.Fatal(err)
	}
	if !assessment.Eligible ||
		strings.Join(assessment.ReusableActionIDs, ",") != "update-npm" ||
		strings.Join(assessment.RemainingActionIDs, ",") != "update-corepack,update-pnpm" {
		t.Fatalf("continuation assessment = %#v", assessment)
	}
}

type functionRunner func(execution.CommandSpec) execution.ProcessResult

func (runner functionRunner) Run(_ context.Context, spec execution.CommandSpec) execution.ProcessResult {
	return runner(spec)
}

func corepackVersionRunner(version string) execution.ProcessRunner {
	return functionRunner(func(spec execution.CommandSpec) execution.ProcessResult {
		if filepath.Base(spec.Executable) == "pnpm" && len(spec.Args) == 1 && spec.Args[0] == "--version" {
			code := 0
			return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: version + "\n"}}
		}
		return execution.ProcessResult{Failure: &execution.ExecutionError{Code: execution.CodeStartFailed, Message: "unexpected fixture command"}}
	})
}
