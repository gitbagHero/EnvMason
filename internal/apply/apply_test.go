package apply

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/assessment"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/report"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

var applyTestTime = time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC)

func TestPrepareBuildsImmutableExecutablePlanWithoutWrites(t *testing.T) {
	t.Parallel()
	service, directory, _ := fixtureService(t)
	prepared, err := service.Prepare(t.Context(), Options{ToolID: "runtime.node", Version: "24.14.0", Online: true})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Plan.SchemaVersion != "0.2.0" || !prepared.Plan.Executable || prepared.Plan.Actions[0].Risk != "R2" || prepared.Plan.Actions[0].TargetVersion != "24.14.0" {
		t.Fatalf("Plan = %#v", prepared.Plan)
	}
	if _, err := os.Stat(service.HistoryRoot); !os.IsNotExist(err) {
		t.Fatalf("prepare created history root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "versions", "node", "v24.14.0")); !os.IsNotExist(err) {
		t.Fatalf("prepare installed target: %v", err)
	}
}

func TestExecuteFixtureInstallsVerifiesAndSecondRunSkips(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("I15 NVM execution is macOS-only and fixture requires bash")
	}
	service, directory, clock := fixtureService(t)
	prepared, err := service.Prepare(context.Background(), Options{ToolID: "runtime.node", Version: "24.14.0", Online: true})
	if err != nil {
		t.Fatal(err)
	}
	*clock = applyTestTime.Add(time.Minute)
	receipt := execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: applyTestTime.Add(30 * time.Second)}
	result, err := service.Execute(t.Context(), prepared, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if result.Record.State != execution.StateCompleted || result.Record.Steps[0].Skipped || result.Record.Steps[0].Before == nil || result.Record.Steps[0].After == nil {
		t.Fatalf("first result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(directory, "versions", "node", "v24.14.0", "bin", "node")); err != nil {
		t.Fatal(err)
	}
	recordJSON, err := execution.MarshalRecord(result.Record)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(recordJSON), directory) || !strings.Contains(string(recordJSON), "[REDACTED]") {
		t.Fatalf("operation record leaked NVM path: %s", recordJSON)
	}
	*clock = applyTestTime.Add(2 * time.Minute)
	receipt.ConfirmedAt = applyTestTime.Add(90 * time.Second)
	second, err := service.Execute(t.Context(), prepared, receipt)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Record.Steps[0].Skipped || second.Record.State != execution.StateCompleted {
		t.Fatalf("second result = %#v", second)
	}
}

func TestExecuteRejectsMismatchedConfirmationBeforeHistoryWrite(t *testing.T) {
	t.Parallel()
	service, _, clock := fixtureService(t)
	prepared, err := service.Prepare(t.Context(), Options{ToolID: "runtime.node", Version: "24.14.0", Online: true})
	if err != nil {
		t.Fatal(err)
	}
	*clock = applyTestTime.Add(time.Minute)
	_, err = service.Execute(t.Context(), prepared, execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ConfirmedAt: applyTestTime.Add(30 * time.Second)})
	if err == nil {
		t.Fatal("mismatched confirmation was accepted")
	}
	if _, statErr := os.Stat(service.HistoryRoot); !os.IsNotExist(statErr) {
		t.Fatalf("rejected confirmation created history: %v", statErr)
	}
}

func TestExecuteRejectsEnvironmentDriftBeforeHistoryWrite(t *testing.T) {
	t.Parallel()
	service, directory, clock := fixtureService(t)
	prepared, err := service.Prepare(t.Context(), Options{ToolID: "runtime.node", Version: "24.14.0", Online: true})
	if err != nil {
		t.Fatal(err)
	}
	drifted := fixtureInventory(directory)
	drifted.System.OSVersion = "15.1"
	service.Scan = func(context.Context) (inventory.Inventory, error) { return drifted, nil }
	*clock = applyTestTime.Add(time.Minute)
	receipt := execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: applyTestTime.Add(30 * time.Second)}
	if _, err := service.Execute(t.Context(), prepared, receipt); err == nil || !strings.Contains(err.Error(), "changed after review") {
		t.Fatalf("environment drift = %v", err)
	}
	if _, statErr := os.Stat(service.HistoryRoot); !os.IsNotExist(statErr) {
		t.Fatalf("environment drift created history: %v", statErr)
	}
}

func TestExecuteRecordsNVMControlFileDriftAsPreconditionFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("I15 NVM execution is macOS-only and fixture requires bash")
	}
	for _, test := range []struct {
		name  string
		path  string
		value []byte
	}{
		{name: "nvm script", path: "nvm.sh", value: []byte("changed\n")},
		{name: "default alias", path: filepath.Join("alias", "default"), value: []byte("20\n")},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, directory, clock := fixtureService(t)
			prepared, err := service.Prepare(t.Context(), Options{ToolID: "runtime.node", Version: "24.14.0", Online: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, test.path), test.value, 0o644); err != nil {
				t.Fatal(err)
			}
			*clock = applyTestTime.Add(time.Minute)
			receipt := execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: applyTestTime.Add(30 * time.Second)}
			result, err := service.Execute(t.Context(), prepared, receipt)
			var failure *execution.ExecutionError
			if !errors.As(err, &failure) || failure.Code != execution.CodePreconditionFailed || result.Record.State != execution.StateFailed {
				t.Fatalf("control-file drift = %#v, %v", result, err)
			}
			if _, statErr := os.Stat(filepath.Join(directory, "versions", "node", "v24.14.0")); !os.IsNotExist(statErr) {
				t.Fatalf("precondition failure installed target: %v", statErr)
			}
		})
	}
}

func TestExecuteRecordsDownloadAndDiskFailures(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("I15 NVM execution is macOS-only and fixture requires bash")
	}
	for _, marker := range []string{"fail-download", "fail-disk"} {
		t.Run(marker, func(t *testing.T) {
			service, directory, clock := fixtureService(t)
			prepared, err := service.Prepare(t.Context(), Options{ToolID: "runtime.node", Version: "24.14.0", Online: true})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, marker), []byte("1"), 0o600); err != nil {
				t.Fatal(err)
			}
			*clock = applyTestTime.Add(time.Minute)
			receipt := execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: applyTestTime.Add(30 * time.Second)}
			result, err := service.Execute(t.Context(), prepared, receipt)
			var failure *execution.ExecutionError
			if !errors.As(err, &failure) || failure.Code != execution.CodeExitNonZero || result.Record.State != execution.StateFailed || result.RecordPath == "" {
				t.Fatalf("failure result = %#v, %v", result, err)
			}
			if _, statErr := os.Stat(filepath.Join(directory, "versions", "node", "v24.14.0")); !os.IsNotExist(statErr) {
				t.Fatalf("failed action produced target: %v", statErr)
			}
		})
	}
}

func TestExecuteCancellationRecordsCancelledAndStopsInstall(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("I15 NVM execution is macOS-only and fixture requires bash")
	}
	service, directory, clock := fixtureService(t)
	prepared, err := service.Prepare(t.Context(), Options{ToolID: "runtime.node", Version: "24.14.0", Online: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "wait-install"), []byte("1"), 0o600); err != nil {
		t.Fatal(err)
	}
	*clock = applyTestTime.Add(time.Minute)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	receipt := execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: prepared.Plan.ID, ConfirmedAt: applyTestTime.Add(30 * time.Second)}
	result, err := service.Execute(ctx, prepared, receipt)
	var failure *execution.ExecutionError
	if !errors.As(err, &failure) || failure.Code != execution.CodeCancelled || result.Record.State != execution.StateCancelled {
		t.Fatalf("cancel result = %#v, %v", result, err)
	}
	if _, statErr := os.Stat(filepath.Join(directory, "versions", "node", "v24.14.0")); !os.IsNotExist(statErr) {
		t.Fatalf("cancelled action produced target: %v", statErr)
	}
}

func fixtureService(t *testing.T) (Service, string, *time.Time) {
	t.Helper()
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "alias"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	createNode(t, directory, "22.0.0")
	nvmScript := `nvm() {
	shift
	target=""
	for argument in "$@"; do case "$argument" in -*) ;; *) target="$argument" ;; esac; done
	if [ -f "$NVM_DIR/fail-download" ]; then return 42; fi
	if [ -f "$NVM_DIR/fail-disk" ]; then return 43; fi
	if [ -f "$NVM_DIR/wait-install" ]; then sleep 5; fi
	destination="$NVM_DIR/versions/node/v$target/bin"
  mkdir -p "$destination"
  printf '#!/bin/sh\nprintf "v%s\\n"\n' "$target" > "$destination/node"
  chmod +x "$destination/node"
}
`
	if err := os.WriteFile(filepath.Join(directory, "nvm.sh"), []byte(nvmScript), 0o644); err != nil {
		t.Fatal(err)
	}
	value := fixtureInventory(directory)
	versions := versiondata.Result{Node: versiondata.NodeData{
		AvailableVersions: []string{"v24.14.0"}, ReleaseIndexFreshness: versiondata.FreshnessFresh,
	}}
	assessmentResult := report.AssessmentResult{Inventory: value, Versions: versions, Policy: assessment.DefaultPolicy()}
	clock := applyTestTime
	environment := map[string]string{"HOME": t.TempDir(), "NVM_DIR": directory, "TMPDIR": t.TempDir()}
	service := Service{
		GOOS: "darwin", Now: func() time.Time { return clock },
		LookupEnv: func(key string) (string, bool) { value, ok := environment[key]; return value, ok },
		Assess:    func(context.Context, report.Options) (report.AssessmentResult, error) { return assessmentResult, nil },
		Scan:      func(context.Context) (inventory.Inventory, error) { return value, nil },
		Runner:    execution.OSRunner{}, HistoryRoot: filepath.Join(t.TempDir(), "operations"),
	}
	return service, directory, &clock
}

func fixtureInventory(directory string) inventory.Inventory {
	return inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion, GeneratedAt: applyTestTime,
		System: inventory.System{
			OS: inventory.OSMacOS, OSVersion: "15.0", OSBuild: "test", Architecture: inventory.ArchitectureARM64,
			ProcessArchitecture: inventory.ArchitectureARM64, TranslationState: inventory.TranslationStateNative,
			Shell:       inventory.Shell{LoginPath: "/bin/zsh", LoginName: "zsh", InvokingPath: "/bin/zsh", InvokingName: "zsh"},
			PathEntries: []inventory.PathEntry{}, Sources: []inventory.SourceMetadata{},
		},
		Tools: []inventory.Tool{{ID: "runtime.node", DisplayName: "Node.js", Category: inventory.CategoryRuntime, Installations: []inventory.Installation{{
			ID: "node-nvm-22", Version: "v22.0.0", NormalizedVersion: "22.0.0", Path: filepath.Join(directory, "versions", "node", "v22.0.0", "bin", "node"),
			Architecture: inventory.ArchitectureARM64, Manager: "nvm", ActiveState: inventory.ActiveStateActive,
			DefaultState: inventory.DefaultStateDefault, InstallReason: inventory.InstallReasonDirect, Sources: []inventory.SourceMetadata{},
		}}}}, Findings: []inventory.Finding{},
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
