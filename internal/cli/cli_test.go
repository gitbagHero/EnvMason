package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"

	applypkg "github.com/gitbagHero/EnvMason/internal/apply"
	"github.com/gitbagHero/EnvMason/internal/buildinfo"
	defaultpkg "github.com/gitbagHero/EnvMason/internal/defaultversion"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/inventory"
	planpkg "github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/report"
)

var testInfo = buildinfo.Info{
	Version:   "1.2.3-test",
	Commit:    "abc123",
	BuildTime: "2026-07-15T12:00:00Z",
	GoVersion: "go1.25.0",
	Target:    "testos/testarch",
}

func TestHelpEntryPoints(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no arguments"},
		{name: "help command", args: []string{"help"}},
		{name: "short help flag", args: []string{"-h"}},
		{name: "long help flag", args: []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := executeForTest(tt.args)
			if code != ExitSuccess {
				t.Fatalf("exit code = %d, want %d", code, ExitSuccess)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			for _, want := range []string{"Usage:", "envmason [command]", "--version", "version", "report", "plan", "apply", "default"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, stdout)
				}
			}
			if strings.Contains(stdout, "completion") {
				t.Errorf("help exposes out-of-scope completion command:\n%s", stdout)
			}
			if strings.Contains(stdout, "-v, --version") {
				t.Errorf("help exposes reserved -v shorthand:\n%s", stdout)
			}
		})
	}
}

func TestDefaultSetDryRunAndExplicitR3Confirmation(t *testing.T) {
	prepared := defaultPreparedForCLI(t, "set_default")
	confirmed, executed := 0, 0
	deps := commandDependencies{
		prepareDefaultSet: func(context.Context, defaultpkg.SetOptions) (defaultpkg.Prepared, error) { return prepared, nil },
		confirmDefault: func(phrase, id string) (execution.ConfirmationReceipt, error) {
			confirmed++
			if phrase != "set-default" || id != prepared.Plan.ID {
				t.Fatalf("confirmation = %q/%q", phrase, id)
			}
			return execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: id, ConfirmedAt: prepared.Plan.CreatedAt.Add(time.Second)}, nil
		},
		executeDefault: func(context.Context, defaultpkg.Prepared, execution.ConfirmationReceipt) (defaultpkg.Result, error) {
			executed++
			return defaultpkg.Result{Record: execution.Record{ID: "op-00000000000000000000000000000001"}}, nil
		},
	}
	code, stdout, stderr := executeForTestWithDependencies([]string{"default", "set", "--tool", "runtime.node", "--version", "24.14.0", "--dry-run"}, deps)
	if code != ExitSuccess || stderr != "" || confirmed != 0 || executed != 0 || !strings.Contains(stdout, "risk=R3") || !strings.Contains(stdout, "Dry run complete") {
		t.Fatalf("default dry run = %d/%q/%q confirmed=%d executed=%d", code, stdout, stderr, confirmed, executed)
	}
	code, stdout, stderr = executeForTestWithDependencies([]string{"default", "set", "--tool", "runtime.node", "--version", "24.14.0"}, deps)
	if code != ExitSuccess || stderr != "" || confirmed != 1 || executed != 1 || !strings.Contains(stdout, "Type 'set-default "+prepared.Plan.ID+"'") {
		t.Fatalf("default set = %d/%q/%q confirmed=%d executed=%d", code, stdout, stderr, confirmed, executed)
	}
}

func TestDefaultRestoreUsesNewExplicitPlanConfirmation(t *testing.T) {
	prepared := defaultPreparedForCLI(t, "restore_default")
	confirmed, executed := 0, 0
	deps := commandDependencies{
		prepareDefaultRestore: func(_ context.Context, options defaultpkg.RestoreOptions) (defaultpkg.Prepared, error) {
			if options.OperationID != "op-00000000000000000000000000000001" {
				t.Fatalf("restore options = %#v", options)
			}
			return prepared, nil
		},
		confirmDefault: func(phrase, id string) (execution.ConfirmationReceipt, error) {
			confirmed++
			if phrase != "restore-default" || id != prepared.Plan.ID {
				t.Fatalf("confirmation = %q/%q", phrase, id)
			}
			return execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: id, ConfirmedAt: prepared.Plan.CreatedAt.Add(time.Second)}, nil
		},
		executeDefault: func(context.Context, defaultpkg.Prepared, execution.ConfirmationReceipt) (defaultpkg.Result, error) {
			executed++
			return defaultpkg.Result{Record: execution.Record{ID: "op-00000000000000000000000000000002"}}, nil
		},
	}
	code, stdout, stderr := executeForTestWithDependencies([]string{"default", "restore", "--operation", "op-00000000000000000000000000000001"}, deps)
	if code != ExitSuccess || stderr != "" || confirmed != 1 || executed != 1 || !strings.Contains(stdout, "Type 'restore-default "+prepared.Plan.ID+"'") {
		t.Fatalf("default restore = %d/%q/%q confirmed=%d executed=%d", code, stdout, stderr, confirmed, executed)
	}
}

func TestDefaultUnsafeUsageAndFailureRecoverySuggestion(t *testing.T) {
	prepared := defaultPreparedForCLI(t, "set_default")
	before, _ := execution.NewSnapshot(map[string]string{"default_alias_hash": "sha256:" + strings.Repeat("a", 64)})
	after, _ := execution.NewSnapshot(map[string]string{"default_alias_hash": "sha256:" + strings.Repeat("b", 64)})
	executed := 0
	deps := commandDependencies{
		prepareDefaultSet: func(context.Context, defaultpkg.SetOptions) (defaultpkg.Prepared, error) { return prepared, nil },
		confirmDefault: func(_, id string) (execution.ConfirmationReceipt, error) {
			return execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: id, ConfirmedAt: prepared.Plan.CreatedAt.Add(time.Second)}, nil
		},
		executeDefault: func(context.Context, defaultpkg.Prepared, execution.ConfirmationReceipt) (defaultpkg.Result, error) {
			executed++
			return defaultpkg.Result{Record: execution.Record{ID: "op-00000000000000000000000000000001", Steps: []execution.StepRecord{{Before: &before, After: &after}}}}, errors.New("verification failed")
		},
	}
	code, stdout, stderr := executeForTestWithDependencies([]string{"default", "set", "--tool", "runtime.node", "--version", "24.14.0"}, deps)
	if code != ExitFailure || executed != 1 || !strings.Contains(stdout, "default restore --operation op-00000000000000000000000000000001 --dry-run") || !strings.Contains(stderr, "verification failed") {
		t.Fatalf("failed default set = %d/%q/%q executed=%d", code, stdout, stderr, executed)
	}
	code, _, stderr = executeForTestWithDependencies([]string{"default", "set", "--tool", "runtime.node", "--version", "24.14.0", "--yes"}, deps)
	if code != ExitUsage || executed != 1 || !strings.Contains(stderr, "unknown flag: --yes") {
		t.Fatalf("unsafe default flag = %d/%q executed=%d", code, stderr, executed)
	}
	for _, args := range [][]string{
		{"default", "set", "--tool", "runtime.java", "--version", "24.14.0"},
		{"default", "set", "--tool", "runtime.node", "--version", "lts/*"},
		{"default", "restore", "--operation", "bad"},
	} {
		code, _, _ := executeForTestWithDependencies(args, deps)
		if code != ExitUsage {
			t.Fatalf("unsafe usage %q = %d", args, code)
		}
	}
}

func defaultPreparedForCLI(t *testing.T, operation string) defaultpkg.Prepared {
	t.Helper()
	createdAt := time.Date(2026, 7, 20, 8, 0, 0, 0, time.UTC)
	inventoryValue := inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion, GeneratedAt: createdAt,
		System: inventory.System{OS: inventory.OSMacOS, OSVersion: "15.0", Architecture: inventory.ArchitectureARM64},
		Tools: []inventory.Tool{{ID: "runtime.node", Installations: []inventory.Installation{
			{ID: "node-22", Version: "v22.0.0", Path: "$HOME/.nvm/versions/node/v22.0.0/bin/node", Manager: "nvm", ActiveState: inventory.ActiveStateActive, DefaultState: inventory.DefaultStateDefault},
			{ID: "node-24", Version: "v24.14.0", Path: "$HOME/.nvm/versions/node/v24.14.0/bin/node", Manager: "nvm", ActiveState: inventory.ActiveStateInactive, DefaultState: inventory.DefaultStateNonDefault},
		}}},
	}
	digest22 := aliasDigestForCLI("22")
	var value planpkg.Plan
	var err error
	if operation == "set_default" {
		value, err = planpkg.BuildDefaultSet(planpkg.DefaultSetInput{
			Inventory: inventoryValue, CreatedAt: createdAt, TargetVersion: "24.14.0", ScriptDigest: "sha256:" + strings.Repeat("a", 64),
			CurrentAliasDigest: digest22, CurrentAlias: "22", CurrentDefaultVersion: "v22.0.0",
		})
	} else {
		value, err = planpkg.BuildDefaultRestore(planpkg.DefaultRestoreInput{
			Inventory: inventoryValue, CreatedAt: createdAt, ScriptDigest: "sha256:" + strings.Repeat("a", 64),
			CurrentAliasDigest: aliasDigestForCLI("v24.14.0"), CurrentAlias: "v24.14.0", CurrentDefaultVersion: "v24.14.0",
			OriginalAliasDigest: digest22, OriginalAlias: "22", OriginalDefaultVersion: "v22.0.0",
			SourceOperationID: "op-00000000000000000000000000000001", SourcePlanID: "sha256:" + strings.Repeat("b", 64),
		})
	}
	if err != nil {
		t.Fatal(err)
	}
	return defaultpkg.Prepared{Plan: value}
}

func aliasDigestForCLI(value string) string {
	digest := sha256.Sum256([]byte(value + "\n"))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func TestApplyDryRunReviewsPlanWithoutConfirmationOrExecution(t *testing.T) {
	prepared := applyPreparedForCLI(t)
	confirmed, executed := 0, 0
	deps := commandDependencies{
		prepareApply: func(context.Context, applypkg.Options) (applypkg.Prepared, error) { return prepared, nil },
		confirmApply: func(string) (execution.ConfirmationReceipt, error) {
			confirmed++
			return execution.ConfirmationReceipt{}, nil
		},
		executeApply: func(context.Context, applypkg.Prepared, execution.ConfirmationReceipt) (applypkg.Result, error) {
			executed++
			return applypkg.Result{}, nil
		},
	}
	code, stdout, stderr := executeForTestWithDependencies([]string{"apply", "--tool", "runtime.node", "--version", "24.14.0", "--online", "--dry-run"}, deps)
	if code != ExitSuccess || stderr != "" || confirmed != 0 || executed != 0 || !strings.Contains(stdout, "Executable: true") || !strings.Contains(stdout, "Dry run complete") {
		t.Fatalf("dry run = %d/%q/%q, confirmed %d, executed %d", code, stdout, stderr, confirmed, executed)
	}
}

func TestApplyRequiresExactPlanBoundConfirmation(t *testing.T) {
	prepared := applyPreparedForCLI(t)
	executed := 0
	deps := commandDependencies{
		prepareApply: func(context.Context, applypkg.Options) (applypkg.Prepared, error) { return prepared, nil },
		confirmApply: func(id string) (execution.ConfirmationReceipt, error) {
			if id != prepared.Plan.ID {
				t.Fatalf("confirmation ID = %s", id)
			}
			return execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: id, ConfirmedAt: prepared.Plan.CreatedAt.Add(time.Second)}, nil
		},
		executeApply: func(_ context.Context, got applypkg.Prepared, receipt execution.ConfirmationReceipt) (applypkg.Result, error) {
			executed++
			if got.Plan.ID != prepared.Plan.ID || receipt.ConfirmedPlanID != prepared.Plan.ID {
				t.Fatal("execution was not Plan-bound")
			}
			return applypkg.Result{Record: execution.Record{ID: "op-00000000000000000000000000000001"}, RecordPath: "/tmp/op.json"}, nil
		},
	}
	code, stdout, stderr := executeForTestWithDependencies([]string{"apply", "--tool", "runtime.node", "--version", "24.14.0", "--online"}, deps)
	if code != ExitSuccess || stderr != "" || executed != 1 || !strings.Contains(stdout, "Type 'apply "+prepared.Plan.ID+"'") || !strings.Contains(stdout, "completed and verified") {
		t.Fatalf("apply = %d/%q/%q, executed %d", code, stdout, stderr, executed)
	}
}

func TestApplyRejectionAndUnsafeFlagsNeverExecute(t *testing.T) {
	prepared := applyPreparedForCLI(t)
	executed := 0
	deps := commandDependencies{
		prepareApply: func(context.Context, applypkg.Options) (applypkg.Prepared, error) { return prepared, nil },
		confirmApply: func(string) (execution.ConfirmationReceipt, error) {
			return execution.ConfirmationReceipt{}, errors.New("Plan was not confirmed; no action was executed")
		},
		executeApply: func(context.Context, applypkg.Prepared, execution.ConfirmationReceipt) (applypkg.Result, error) {
			executed++
			return applypkg.Result{}, nil
		},
	}
	code, _, stderr := executeForTestWithDependencies([]string{"apply", "--tool", "runtime.node", "--version", "24.14.0", "--online"}, deps)
	if code != ExitFailure || executed != 0 || !strings.Contains(stderr, "not confirmed") {
		t.Fatalf("rejection = %d/%q, executed %d", code, stderr, executed)
	}
	code, _, stderr = executeForTestWithDependencies([]string{"apply", "--tool", "runtime.node", "--version", "24.14.0", "--online", "--yes"}, deps)
	if code != ExitUsage || executed != 0 || !strings.Contains(stderr, "unknown flag: --yes") {
		t.Fatalf("unsafe flag = %d/%q, executed %d", code, stderr, executed)
	}
}

func TestApplyRejectsMissingOnlineAndUnsupportedToolBeforePreparation(t *testing.T) {
	preparedCalls := 0
	deps := commandDependencies{
		prepareApply: func(context.Context, applypkg.Options) (applypkg.Prepared, error) {
			preparedCalls++
			return applypkg.Prepared{}, nil
		},
		executeApply: func(context.Context, applypkg.Prepared, execution.ConfirmationReceipt) (applypkg.Result, error) {
			return applypkg.Result{}, nil
		},
		confirmApply: func(string) (execution.ConfirmationReceipt, error) { return execution.ConfirmationReceipt{}, nil },
	}
	for _, args := range [][]string{
		{"apply", "--tool", "runtime.node", "--version", "24.14.0"},
		{"apply", "--tool", "runtime.java", "--version", "24.14.0", "--online"},
		{"apply", "--tool", "runtime.node", "--online"},
	} {
		code, _, _ := executeForTestWithDependencies(args, deps)
		if code != ExitUsage {
			t.Fatalf("usage %q = %d", args, code)
		}
	}
	if preparedCalls != 0 {
		t.Fatalf("prepare called %d times", preparedCalls)
	}
}

func applyPreparedForCLI(t *testing.T) applypkg.Prepared {
	t.Helper()
	value, err := planpkg.BuildSelfTest(planpkg.SelfTestInput{CreatedAt: time.Date(2026, 7, 17, 10, 0, 0, 0, time.UTC), OS: "darwin", OSVersion: "15.0", Architecture: "arm64"})
	if err != nil {
		t.Fatal(err)
	}
	return applypkg.Prepared{Plan: value}
}

func TestPlanCommandPassesOnlyConfirmedPreviewOptions(t *testing.T) {
	var received planpkg.Options
	code, stdout, stderr := executeForTestWithDependencies(
		[]string{"plan", "--tool", "runtime.node", "--online", "--format", "json", "--project", "/workspace", "--exclude", "archived", "--policy", "/workspace/policy.json"},
		commandDependencies{generateReport: report.Generate, generatePlan: func(_ context.Context, options planpkg.Options) ([]byte, error) {
			received = options
			return []byte("{\"executable\":false}\n"), nil
		}},
	)
	if code != ExitSuccess || stdout != "{\"executable\":false}\n" || stderr != "" {
		t.Fatalf("plan result = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if received.ToolID != "runtime.node" || received.Format != planpkg.FormatJSON || !received.Online || received.PolicyPath != "/workspace/policy.json" || len(received.Projects) != 1 || len(received.Excludes) != 1 {
		t.Fatalf("plan options = %#v", received)
	}
}

func TestPlanCommandRejectsUnsafeOrUnsupportedUsageBeforeGeneration(t *testing.T) {
	called := 0
	deps := commandDependencies{generateReport: report.Generate, generatePlan: func(context.Context, planpkg.Options) ([]byte, error) {
		called++
		return nil, errors.New("unexpected")
	}}
	for _, args := range [][]string{
		{"plan", "--tool", "runtime.node"},
		{"plan", "--tool", "runtime.java", "--online"},
		{"plan", "--tool", "runtime.node", "--online", "--format", "markdown"},
		{"plan", "--tool", "runtime.node", "--online", "--exclude", "archived"},
	} {
		code, stdout, stderr := executeForTestWithDependencies(args, deps)
		if code != ExitUsage || stdout != "" || !strings.Contains(stderr, "for usage") {
			t.Fatalf("unsafe usage %q = %d/%q/%q", args, code, stdout, stderr)
		}
	}
	if called != 0 {
		t.Fatalf("generator called %d times for invalid usage", called)
	}
}

func TestPlanOperationalFailureUsesExitOne(t *testing.T) {
	code, stdout, stderr := executeForTestWithDependencies([]string{"plan", "--tool", "runtime.node", "--online"}, commandDependencies{
		generateReport: report.Generate,
		generatePlan: func(context.Context, planpkg.Options) ([]byte, error) {
			return nil, errors.New("no eligible recommendation")
		},
	})
	if code != ExitFailure || stdout != "" || !strings.Contains(stderr, "no eligible recommendation") || strings.Contains(stderr, "for usage") {
		t.Fatalf("operational plan error = %d/%q/%q", code, stdout, stderr)
	}
}

func TestReportCommandPassesConfirmedOptions(t *testing.T) {
	var received report.Options
	code, stdout, stderr := executeForTestWithDependencies(
		[]string{"report", "--format", "markdown", "--category", "runtime", "--category", "ecosystem", "--severity", "warning", "--online", "--project", "/workspace", "--exclude", "archived", "--policy", "/workspace/policy.json"},
		commandDependencies{generateReport: func(_ context.Context, options report.Options) ([]byte, error) {
			received = options
			return []byte("# report\n"), nil
		}},
	)
	if code != ExitSuccess || stdout != "# report\n" || stderr != "" {
		t.Fatalf("report result = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	if received.Format != report.FormatMarkdown {
		t.Fatalf("format = %q", received.Format)
	}
	if got := strings.Join([]string{string(received.Categories[0]), string(received.Categories[1])}, ","); got != "runtime,ecosystem" {
		t.Fatalf("categories = %q", got)
	}
	if len(received.Severities) != 1 || received.Severities[0] != "warning" {
		t.Fatalf("severities = %#v", received.Severities)
	}
	if !received.Online {
		t.Fatal("--online was not passed to report generation")
	}
	if len(received.Projects) != 1 || received.Projects[0] != "/workspace" || len(received.Excludes) != 1 || received.Excludes[0] != "archived" {
		t.Fatalf("project options = %#v / %#v", received.Projects, received.Excludes)
	}
	if received.PolicyPath != "/workspace/policy.json" {
		t.Fatalf("policy path = %q", received.PolicyPath)
	}
}

func TestReportCommandDefaultsToSummary(t *testing.T) {
	var received report.Options
	code, _, stderr := executeForTestWithDependencies([]string{"report"}, commandDependencies{generateReport: func(_ context.Context, options report.Options) ([]byte, error) {
		received = options
		return []byte("summary\n"), nil
	}})
	if code != ExitSuccess || stderr != "" {
		t.Fatalf("report result = code %d, stderr %q", code, stderr)
	}
	if received.Format != report.FormatSummary || len(received.Categories) != 0 || len(received.Severities) != 0 || received.Online {
		t.Fatalf("default options = %#v", received)
	}
}

func TestReportUsageAndOperationalErrorsHaveDifferentExitCodes(t *testing.T) {
	deps := commandDependencies{generateReport: func(context.Context, report.Options) ([]byte, error) {
		return nil, errors.New("scan unavailable")
	}}
	code, stdout, stderr := executeForTestWithDependencies([]string{"report", "--format", "yaml"}, deps)
	if code != ExitUsage || stdout != "" || !strings.Contains(stderr, "unsupported report format") || !strings.Contains(stderr, "Run 'envmason help' for usage.") {
		t.Fatalf("usage error = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, stdout, stderr = executeForTestWithDependencies([]string{"report"}, deps)
	if code != ExitFailure || stdout != "" || !strings.Contains(stderr, "scan unavailable") || strings.Contains(stderr, "for usage") {
		t.Fatalf("operational error = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, stdout, stderr = executeForTestWithDependencies([]string{"report", "--exclude", "archived"}, deps)
	if code != ExitUsage || stdout != "" || !strings.Contains(stderr, "--exclude requires") {
		t.Fatalf("exclude usage error = code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestVersionEntryPoints(t *testing.T) {
	want := "envmason 1.2.3-test\n" +
		"commit: abc123\n" +
		"built: 2026-07-15T12:00:00Z\n" +
		"go: go1.25.0\n" +
		"target: testos/testarch\n"

	for _, args := range [][]string{{"version"}, {"--version"}} {
		code, stdout, stderr := executeForTest(args)
		if code != ExitSuccess {
			t.Fatalf("Execute(%q) exit code = %d, want %d", args, code, ExitSuccess)
		}
		if stdout != want {
			t.Fatalf("Execute(%q) stdout = %q, want %q", args, stdout, want)
		}
		if stderr != "" {
			t.Fatalf("Execute(%q) stderr = %q, want empty", args, stderr)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "unknown command", args: []string{"scan"}, wantErr: `unknown command "scan" for "envmason"`},
		{name: "version extra argument", args: []string{"version", "extra"}, wantErr: `unknown command "extra"`},
		{name: "unknown flag", args: []string{"--json"}, wantErr: "unknown flag: --json"},
		{name: "reserved short version flag", args: []string{"-v"}, wantErr: "unknown shorthand flag: 'v'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := executeForTest(tt.args)
			if code != ExitUsage {
				t.Fatalf("exit code = %d, want %d", code, ExitUsage)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("stderr does not contain %q: %s", tt.wantErr, stderr)
			}
			if !strings.HasSuffix(stderr, "Run 'envmason help' for usage.\n") {
				t.Errorf("stderr does not end with usage hint: %s", stderr)
			}
		})
	}
}

func executeForTest(args []string) (int, string, string) {
	return executeForTestWithDependencies(args, commandDependencies{generateReport: report.Generate, generatePlan: planpkg.Generate})
}

func executeForTestWithDependencies(args []string, deps commandDependencies) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := execute(args, &stdout, &stderr, testInfo, deps)
	return code, stdout.String(), stderr.String()
}
