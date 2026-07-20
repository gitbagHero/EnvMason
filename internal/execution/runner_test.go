package execution

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestExecutionHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_EXECUTION_HELPER") != "1" {
		return
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 || separator+1 >= len(os.Args) {
		os.Exit(97)
	}
	arguments := os.Args[separator+1:]
	switch arguments[0] {
	case "success":
		fmt.Fprint(os.Stdout, "ok")
	case "failure":
		fmt.Fprint(os.Stderr, "failed")
		os.Exit(7)
	case "sleep":
		time.Sleep(5 * time.Second)
	case "echo":
		_ = json.NewEncoder(os.Stdout).Encode(arguments[1:])
	case "output":
		fmt.Fprint(os.Stdout, strings.Repeat("x", DefaultOutputLimit+1024))
	case "token":
		fmt.Fprintf(os.Stdout, "token=%s", arguments[1])
		fmt.Fprintf(os.Stderr, " secret: %s", arguments[1])
	case "kill":
		process, _ := os.FindProcess(os.Getpid())
		_ = process.Kill()
		time.Sleep(time.Second)
	case "tree-parent":
		command := exec.Command(os.Args[0], "-test.run=^TestExecutionHelperProcess$", "--", "tree-child", arguments[1])
		command.Env = []string{"GO_WANT_EXECUTION_HELPER=1"}
		if err := command.Start(); err != nil {
			os.Exit(96)
		}
		_ = command.Wait()
	case "tree-child":
		time.Sleep(200 * time.Millisecond)
		if err := os.WriteFile(arguments[1], []byte("survived"), 0o600); err != nil {
			os.Exit(95)
		}
	default:
		os.Exit(98)
	}
	os.Exit(0)
}

func TestOSRunnerTerminatesUnixProcessGroup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows I15 execution is unsupported")
	}
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "descendant-survived")
	spec := helperSpec(helperExecutable(t), "tree-parent")
	spec.Args = append(spec.Args, marker)
	spec.Timeout = 30 * time.Millisecond
	spec.TerminateTree = true
	result := (OSRunner{}).Run(t.Context(), spec)
	if result.Failure == nil || result.Failure.Code != CodeTimeout {
		t.Fatalf("timeout = %#v", result)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("descendant was not terminated: %v", err)
	}
}

func TestOSRunnerSuccessAndNonZeroExit(t *testing.T) {
	t.Parallel()
	executable := helperExecutable(t)
	runner := OSRunner{}
	success := runner.Run(context.Background(), helperSpec(executable, "success"))
	if success.Failure != nil || success.ExitCode == nil || *success.ExitCode != 0 || success.Stdout.Text != "ok" {
		t.Fatalf("success result = %#v", success)
	}
	failure := runner.Run(context.Background(), helperSpec(executable, "failure"))
	if failure.Failure == nil || failure.Failure.Code != CodeExitNonZero || failure.ExitCode == nil || *failure.ExitCode != 7 {
		t.Fatalf("failure result = %#v", failure)
	}
}

func TestOSRunnerTimeoutAndCancellation(t *testing.T) {
	t.Parallel()
	executable := helperExecutable(t)
	runner := OSRunner{}
	timeoutSpec := helperSpec(executable, "sleep")
	timeoutSpec.Timeout = 25 * time.Millisecond
	if result := runner.Run(context.Background(), timeoutSpec); result.Failure == nil || result.Failure.Code != CodeTimeout {
		t.Fatalf("timeout result = %#v", result)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if result := runner.Run(ctx, helperSpec(executable, "sleep")); result.Failure == nil || result.Failure.Code != CodeCancelled {
		t.Fatalf("cancel result = %#v", result)
	}
}

func TestOSRunnerPassesArgumentsLiterallyWithoutInjection(t *testing.T) {
	t.Parallel()
	executable := helperExecutable(t)
	marker := filepath.Join(t.TempDir(), "injected")
	arguments := []string{"hello world", "中文🙂", "; touch " + marker, "$(touch " + marker + ")", "a&b|c"}
	spec := helperSpec(executable, "echo")
	spec.Args = append(spec.Args, arguments...)
	result := (OSRunner{}).Run(context.Background(), spec)
	if result.Failure != nil {
		t.Fatal(result.Failure)
	}
	var actual []string
	if err := json.Unmarshal([]byte(result.Stdout.Text), &actual); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(actual) != fmt.Sprint(arguments) {
		t.Fatalf("args = %#v, want %#v", actual, arguments)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("injection marker exists: %v", err)
	}
}

func TestOSRunnerBoundsOutputAndClassifiesAbnormalExit(t *testing.T) {
	t.Parallel()
	executable := helperExecutable(t)
	runner := OSRunner{}
	output := runner.Run(context.Background(), helperSpec(executable, "output"))
	if output.Failure != nil || len(output.Stdout.Text) != DefaultOutputLimit || !output.Stdout.Truncated {
		t.Fatalf("bounded output = len %d, truncated %v, failure %v", len(output.Stdout.Text), output.Stdout.Truncated, output.Failure)
	}
	abnormal := runner.Run(context.Background(), helperSpec(executable, "kill"))
	if abnormal.Failure == nil {
		t.Fatal("self-killed process was reported successful")
	}
	if runtime.GOOS != "windows" && abnormal.Failure.Code != CodeAbnormalExit {
		t.Fatalf("abnormal failure = %v", abnormal.Failure)
	}
}

func TestOSRunnerRejectsUnsafeSpecification(t *testing.T) {
	t.Parallel()
	for _, spec := range []CommandSpec{
		{Executable: "relative", Timeout: time.Second},
		{Executable: helperExecutable(t), Timeout: 0},
		{Executable: helperExecutable(t), Timeout: MaximumTimeout + time.Second},
	} {
		if result := (OSRunner{}).Run(context.Background(), spec); result.Failure == nil || result.Failure.Code != CodeStartFailed {
			t.Fatalf("unsafe spec result = %#v", result)
		}
	}
	missing := CommandSpec{Executable: filepath.Join(t.TempDir(), "missing-executable"), Timeout: time.Second}
	if result := (OSRunner{}).Run(context.Background(), missing); result.Failure == nil || result.Failure.Code != CodeStartFailed {
		t.Fatalf("missing executable result = %#v", result)
	}
}

func TestSelfTestDefinitionUsesOnlyFixedVersionAction(t *testing.T) {
	t.Parallel()
	executable := helperExecutable(t)
	definition := SelfTestDefinition(executable)
	spec, err := definition.Build(planActionForRegistryTest())
	if err != nil {
		t.Fatal(err)
	}
	if spec.Executable != executable || len(spec.Args) != 1 || spec.Args[0] != "version" || spec.Timeout <= 0 {
		t.Fatalf("self-test spec = %#v", spec)
	}
	exitCode := 0
	if err := definition.Verify(context.Background(), planActionForRegistryTest(), ProcessResult{ExitCode: &exitCode, Stdout: CapturedOutput{Text: "envmason 0.0.1-i14\n"}}); err != nil {
		t.Fatal(err)
	}
}

func planActionForRegistryTest() plan.Action {
	return plan.Action{ToolID: "internal.executor", Operation: "self_test", Adapter: "builtin", Risk: plan.RiskR1}
}

func helperExecutable(t *testing.T) string {
	t.Helper()
	value, err := filepath.Abs(os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func helperSpec(executable, scenario string) CommandSpec {
	return CommandSpec{
		Executable:  executable,
		Args:        []string{"-test.run=^TestExecutionHelperProcess$", "--", scenario},
		Environment: []string{"GO_WANT_EXECUTION_HELPER=1"},
		Timeout:     2 * time.Second,
	}
}
