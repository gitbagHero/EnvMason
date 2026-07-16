package macos

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

var fixtureTime = time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)

type commandResponse struct {
	output string
	err    error
}

type fakeRunner struct {
	responses map[string]commandResponse
	calls     []string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) (string, error) {
	key := commandKey(name, args...)
	r.calls = append(r.calls, key)
	response, ok := r.responses[key]
	if !ok {
		return "", errors.New("unexpected command")
	}
	return response.output, response.err
}

func TestDiscoverAppleSiliconNative(t *testing.T) {
	t.Parallel()

	runner := successfulRunner("0", "/bin/zsh")
	environment := map[string]string{
		"SHELL":        "/bin/zsh",
		"PATH":         "/opt/homebrew/bin:/missing:/opt/homebrew/bin:/Users/alice/bin:relative:",
		"HOME":         "/Users/alice",
		"SECRET_TOKEN": "must-not-be-read",
	}
	var lookedUp []string
	lookup := func(name string) (string, bool) {
		lookedUp = append(lookedUp, name)
		value, ok := environment[name]
		return value, ok
	}
	stat := func(path string) (fs.FileInfo, error) {
		switch path {
		case "/opt/homebrew/bin", "/Users/alice/bin":
			return nil, nil
		case "/missing":
			return nil, fs.ErrNotExist
		default:
			return nil, fs.ErrPermission
		}
	}

	result, err := discover(context.Background(), dependencies{
		runner: runner, lookupEnv: lookup, stat: stat, now: func() time.Time { return fixtureTime },
		goos: "darwin", goarch: "arm64", parentPID: 42,
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.System.OSVersion != "15.7.4" || result.System.OSBuild != "24G517" {
		t.Fatalf("OS version/build = %q/%q", result.System.OSVersion, result.System.OSBuild)
	}
	if result.System.Architecture != inventory.ArchitectureARM64 || result.System.ProcessArchitecture != inventory.ArchitectureARM64 {
		t.Fatalf("system/process architecture = %q/%q", result.System.Architecture, result.System.ProcessArchitecture)
	}
	if result.System.TranslationState != inventory.TranslationStateNative {
		t.Fatalf("translation state = %q", result.System.TranslationState)
	}
	if result.System.Shell.LoginName != "zsh" || result.System.Shell.InvokingName != "zsh" {
		t.Fatalf("shell = %#v", result.System.Shell)
	}

	entries := result.System.PathEntries
	if len(entries) != 6 {
		t.Fatalf("PATH entries = %d, want 6", len(entries))
	}
	if entries[0].Position != 0 || entries[0].State != inventory.PathStateExists || !entries[0].Duplicate {
		t.Fatalf("first PATH entry = %#v", entries[0])
	}
	if entries[1].State != inventory.PathStateMissing {
		t.Fatalf("missing PATH state = %q", entries[1].State)
	}
	if entries[3].Value != "$HOME/bin" || entries[3].State != inventory.PathStateExists {
		t.Fatalf("home PATH entry = %#v", entries[3])
	}
	if entries[4].State != inventory.PathStateUnknown || entries[5].State != inventory.PathStateUnknown {
		t.Fatalf("relative/empty PATH states = %q/%q", entries[4].State, entries[5].State)
	}

	sort.Strings(lookedUp)
	if got := strings.Join(lookedUp, ","); got != "HOME,PATH,SHELL" {
		t.Fatalf("environment lookups = %q", got)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("findings = %#v", result.Findings)
	}
	assertReadOnlyCommands(t, runner.calls)
	assertValidInventory(t, result)
}

func TestDiscoverRosettaProcess(t *testing.T) {
	t.Parallel()

	runner := successfulRunner("1", "/bin/zsh")
	result, err := discover(context.Background(), fixtureDependencies(runner, "amd64"))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.System.Architecture != inventory.ArchitectureARM64 {
		t.Fatalf("system architecture = %q", result.System.Architecture)
	}
	if result.System.ProcessArchitecture != inventory.ArchitectureAMD64 {
		t.Fatalf("process architecture = %q", result.System.ProcessArchitecture)
	}
	if result.System.TranslationState != inventory.TranslationStateTranslated {
		t.Fatalf("translation state = %q", result.System.TranslationState)
	}
}

func TestDiscoverIntelNativeProcess(t *testing.T) {
	t.Parallel()

	runner := successfulRunner("", "/bin/bash")
	runner.responses[commandKey("sysctl", "-n", "hw.optional.arm64")] = commandResponse{output: "0"}
	runner.responses[commandKey("sysctl", "-n", "hw.machine")] = commandResponse{output: "x86_64"}
	deps := fixtureDependencies(runner, "amd64")
	deps.lookupEnv = environmentLookup(map[string]string{"SHELL": "/bin/bash", "PATH": "/usr/bin", "HOME": "/Users/test"})
	result, err := discover(context.Background(), deps)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.System.Architecture != inventory.ArchitectureAMD64 || result.System.TranslationState != inventory.TranslationStateNative {
		t.Fatalf("architecture/translation = %q/%q", result.System.Architecture, result.System.TranslationState)
	}
}

func TestDiscoverDoesNotMislabelParentProcessAsShell(t *testing.T) {
	t.Parallel()

	runner := successfulRunner("0", "/Applications/Codex.app/Contents/MacOS/Codex")
	result, err := discover(context.Background(), fixtureDependencies(runner, "arm64"))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.System.Shell.InvokingPath != unknown || result.System.Shell.InvokingName != unknown {
		t.Fatalf("invoking shell = %#v", result.System.Shell)
	}
}

func TestDiscoverSanitizesProbeFailures(t *testing.T) {
	t.Parallel()

	const secret = "envmason-test-token-should-not-leak"
	runner := &fakeRunner{responses: make(map[string]commandResponse)}
	for _, call := range expectedCommands("/bin/zsh") {
		runner.responses[call] = commandResponse{err: errors.New(secret)}
	}
	deps := fixtureDependencies(runner, "arm64")
	deps.lookupEnv = environmentLookup(map[string]string{
		"SHELL": "/bin/zsh", "PATH": "/usr/bin", "HOME": "/Users/test", "TOKEN": secret,
	})
	result, err := discover(context.Background(), deps)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	if strings.Contains(string(data), secret) {
		t.Fatal("secret leaked into discovery result")
	}
	if len(result.Findings) != len(expectedCommands("/bin/zsh")) {
		t.Fatalf("findings = %d, want %d", len(result.Findings), len(expectedCommands("/bin/zsh")))
	}
	if result.System.OSVersion != unknown || result.System.Architecture != inventory.ArchitectureUnknown {
		t.Fatalf("failed probe fallback = %q/%q", result.System.OSVersion, result.System.Architecture)
	}
}

func TestDiscoverRejectsOtherPlatforms(t *testing.T) {
	t.Parallel()

	_, err := discover(context.Background(), dependencies{goos: "linux"})
	if err == nil {
		t.Fatal("discover unexpectedly accepted a non-macOS platform")
	}
}

func TestDiscoverRealMacOS(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("real macOS acceptance requires macOS")
	}

	result, err := Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if result.System.OS != inventory.OSMacOS || result.System.OSVersion == unknown {
		t.Fatalf("OS = %q %q", result.System.OS, result.System.OSVersion)
	}
	if result.System.Architecture == inventory.ArchitectureUnknown || result.System.ProcessArchitecture == inventory.ArchitectureUnknown {
		t.Fatalf("architecture = %q/%q", result.System.Architecture, result.System.ProcessArchitecture)
	}
	assertValidInventory(t, result)
}

func successfulRunner(translated, invokingShell string) *fakeRunner {
	return &fakeRunner{responses: map[string]commandResponse{
		commandKey("sw_vers", "--productVersion"):             {output: "15.7.4"},
		commandKey("sw_vers", "--buildVersion"):               {output: "24G517"},
		commandKey("sysctl", "-n", "hw.optional.arm64"):       {output: "1"},
		commandKey("sysctl", "-n", "hw.machine"):              {output: "arm64"},
		commandKey("sysctl", "-in", "sysctl.proc_translated"): {output: translated},
		commandKey("ps", "-p", "42", "-o", "comm="):           {output: invokingShell},
	}}
}

func fixtureDependencies(runner *fakeRunner, goarch string) dependencies {
	return dependencies{
		runner: runner,
		lookupEnv: environmentLookup(map[string]string{
			"SHELL": "/bin/zsh", "PATH": "/usr/bin", "HOME": "/Users/test",
		}),
		stat: func(string) (fs.FileInfo, error) { return nil, nil },
		now:  func() time.Time { return fixtureTime },
		goos: "darwin", goarch: goarch, parentPID: 42,
	}
}

func environmentLookup(values map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func commandKey(name string, args ...string) string {
	return strings.Join(append([]string{name}, args...), "\x00")
}

func expectedCommands(_ string) []string {
	return []string{
		commandKey("sw_vers", "--productVersion"),
		commandKey("sw_vers", "--buildVersion"),
		commandKey("sysctl", "-n", "hw.optional.arm64"),
		commandKey("sysctl", "-n", "hw.machine"),
		commandKey("sysctl", "-in", "sysctl.proc_translated"),
		commandKey("ps", "-p", "42", "-o", "comm="),
	}
}

func assertReadOnlyCommands(t *testing.T, calls []string) {
	t.Helper()
	want := expectedCommands("/bin/zsh")
	if len(calls) != len(want) {
		t.Fatalf("command count = %d, want %d", len(calls), len(want))
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("command %d = %q, want %q", i, calls[i], want[i])
		}
		if strings.Contains(calls[i], "=") && !strings.HasSuffix(calls[i], "comm=") {
			t.Fatalf("command may contain a sysctl assignment: %q", calls[i])
		}
	}
}

func assertValidInventory(t *testing.T, result Result) {
	t.Helper()
	_, err := inventory.Marshal(inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion,
		GeneratedAt:   fixtureTime,
		System:        result.System,
		Tools:         []inventory.Tool{},
		Findings:      result.Findings,
	})
	if err != nil {
		t.Fatalf("discovery result violates inventory schema: %v", err)
	}
}
