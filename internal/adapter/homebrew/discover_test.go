package homebrew

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

var fixtureTime = time.Date(2026, 7, 16, 8, 0, 0, 0, time.UTC)

type recordedCall struct {
	path string
	args []string
	env  map[string]string
}

type fakeRunner struct {
	responses map[string]commandOutput
	errors    map[string]error
	calls     []recordedCall
}

func (r *fakeRunner) Run(_ context.Context, path string, args []string, environment map[string]string) (commandOutput, error) {
	key := strings.Join(args, "\x00")
	r.calls = append(r.calls, recordedCall{path: path, args: append([]string{}, args...), env: cloneMap(environment)})
	if err := r.errors[key]; err != nil {
		return r.responses[key], err
	}
	output, ok := r.responses[key]
	if !ok {
		return commandOutput{}, errors.New("unexpected command")
	}
	return output, nil
}

func TestDiscoverReportsHomebrewNotInstalled(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	result, err := discover(context.Background(), fixtureRequest(root), dependencies{runner: &fakeRunner{}})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.State != StateNotInstalled || result.Version != unknown || len(result.Tools) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestDiscoverMapsReadOnlyHomebrewJSON(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	brewPath := createExecutable(t, root, "brew")
	gitPath := createExecutable(t, root, "git")
	brewPath = resolvedPath(t, brewPath)
	gitPath = resolvedPath(t, gitPath)
	runner := successfulFixtureRunner(t)
	result, err := discover(context.Background(), fixtureRequest(root), dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}

	if result.State != StateInstalled || result.Version != "6.0.9" || result.Prefix != "/opt/homebrew" {
		t.Fatalf("state/version/prefix = %q/%q/%q", result.State, result.Version, result.Prefix)
	}
	if result.Architecture != inventory.ArchitectureARM64 {
		t.Fatalf("architecture = %q", result.Architecture)
	}
	if result.Origin != "https://example.test/homebrew/brew.git" {
		t.Fatalf("sanitized origin = %q", result.Origin)
	}
	if len(result.Tools) != 3 || len(result.Outdated) != 2 {
		t.Fatalf("tools/outdated = %d/%d", len(result.Tools), len(result.Outdated))
	}

	node := findTool(t, result.Tools, "homebrew.formula.node")
	if len(node.Installations) != 2 || node.Installations[0].InstallReason != inventory.InstallReasonDirect {
		t.Fatalf("node installations = %#v", node.Installations)
	}
	if node.Installations[1].ActiveState != inventory.ActiveStateActive || node.Installations[1].DefaultState != inventory.DefaultStateDefault {
		t.Fatalf("active node installation = %#v", node.Installations[1])
	}
	dependency := findTool(t, result.Tools, "homebrew.formula.icu4c-77")
	if dependency.Installations[0].InstallReason != inventory.InstallReasonDependency {
		t.Fatalf("dependency reason = %q", dependency.Installations[0].InstallReason)
	}
	cask := findTool(t, result.Tools, "homebrew.cask.visual-studio-code")
	if cask.Installations[0].Path != "/Applications/Visual Studio Code.app" {
		t.Fatalf("cask path = %q", cask.Installations[0].Path)
	}
	outdatedCask := findOutdated(t, result.Outdated, PackageCask, "visual-studio-code")
	if !outdatedCask.Pinned || outdatedCask.PinnedVersion != "1.102.0" {
		t.Fatalf("outdated cask = %#v", outdatedCask)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("findings = %#v", result.Findings)
	}
	assertSchemaCompatibleTools(t, result.Tools)
	assertReadOnlyCalls(t, runner.calls, brewPath, gitPath)
}

func TestDiscoverContinuesAfterFailuresAndDoesNotLeakOutput(t *testing.T) {
	t.Parallel()

	const secret = "homebrew-secret-must-not-leak"
	root := t.TempDir()
	createExecutable(t, root, "brew")
	createExecutable(t, root, "git")
	runner := successfulFixtureRunner(t)
	runner.errors[strings.Join([]string{"--prefix"}, "\x00")] = errors.New(secret)
	runner.responses[strings.Join([]string{"--prefix"}, "\x00")] = commandOutput{stderr: "another process already holds the lock: " + secret}
	runner.responses[strings.Join([]string{"info", "--json=v2", "--installed"}, "\x00")] = commandOutput{stdout: `{invalid`}

	result, err := discover(context.Background(), fixtureRequest(root), dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	data, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		t.Fatalf("marshal: %v", marshalErr)
	}
	if strings.Contains(string(data), secret) {
		t.Fatal("command output or error leaked into result")
	}
	assertFinding(t, result.Findings, "HOMEBREW_LOCKED")
	assertFinding(t, result.Findings, "HOMEBREW_INFO_JSON_INVALID")
	if len(result.Outdated) != 2 {
		t.Fatalf("outdated query did not continue: %#v", result.Outdated)
	}
}

func TestPrefixArchitectureFixtures(t *testing.T) {
	t.Parallel()

	for prefix, want := range map[string]inventory.Architecture{
		"/opt/homebrew": inventory.ArchitectureARM64,
		"/usr/local":    inventory.ArchitectureAMD64,
	} {
		if got := inferArchitecture(prefix, []inventory.Architecture{inventory.ArchitectureUnknown}, inventory.ArchitectureUnknown); got != want {
			t.Fatalf("inferArchitecture(%q) = %q, want %q", prefix, got, want)
		}
	}
}

func TestParsersRejectTrailingJSONAndSupportCaskVersionArray(t *testing.T) {
	t.Parallel()

	source := sourceMetadata("fixture", fixtureTime, inventory.ConfidenceHigh)
	if _, err := parseInfo([]byte(`{"formulae":[],"casks":[]} {}`), "", "", "", inventory.ArchitectureARM64, source); err == nil {
		t.Fatal("parseInfo accepted trailing JSON")
	}
	tools, err := parseInfo([]byte(`{"formulae":[],"casks":[{"token":"demo","installed":["1","2"]}]}`), "", "/opt/homebrew/Caskroom", "", inventory.ArchitectureARM64, source)
	if err != nil || len(tools) != 1 || len(tools[0].Installations) != 2 {
		t.Fatalf("cask version array = %#v, err %v", tools, err)
	}
}

func TestDiscoverRealHomebrewReadOnly(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("real Homebrew acceptance requires macOS")
	}
	path := os.Getenv("PATH")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	home, _ := os.UserHomeDir()
	result, err := Discover(context.Background(), Request{
		PathDirectories: strings.Split(path, string(os.PathListSeparator)), WorkingDirectory: workingDirectory,
		Home: home, CollectedAt: time.Now().UTC(), ProcessArchitecture: inventory.Architecture(runtime.GOARCH),
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if result.State == StateInstalled {
		if result.Version == unknown || result.Prefix == unknown || result.DataFormat != JSONFormatV2 {
			t.Fatalf("installed result incomplete: %#v", result)
		}
		if len(result.Tools) == 0 {
			t.Fatal("real Homebrew returned no installed formulae or casks")
		}
	}
}

func successfulFixtureRunner(t *testing.T) *fakeRunner {
	t.Helper()
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("read fixture: %v", err)
		}
		return string(data)
	}
	return &fakeRunner{responses: map[string]commandOutput{
		"--version":    {stdout: "Homebrew 6.0.9\nHomebrew/homebrew-core"},
		"--prefix":     {stdout: "/opt/homebrew"},
		"--repository": {stdout: "/opt/homebrew"},
		"--cellar":     {stdout: "/opt/homebrew/Cellar"},
		"--caskroom":   {stdout: "/opt/homebrew/Caskroom"},
		strings.Join([]string{"-C", "/opt/homebrew", "remote", "get-url", "origin"}, "\x00"): {stdout: "https://user:token@example.test/homebrew/brew.git?secret=yes#fragment"},
		strings.Join([]string{"info", "--json=v2", "--installed"}, "\x00"):                   {stdout: read("info.json")},
		strings.Join([]string{"outdated", "--json=v2"}, "\x00"):                              {stdout: read("outdated.json")},
	}, errors: make(map[string]error)}
}

func fixtureRequest(root string) Request {
	return Request{PathDirectories: []string{root}, WorkingDirectory: root, Home: root, CollectedAt: fixtureTime, ProcessArchitecture: inventory.ArchitectureARM64}
}

func createExecutable(t *testing.T, directory, name string) string {
	t.Helper()
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 99\n"), 0o755); err != nil {
		t.Fatalf("create executable: %v", err)
	}
	return path
}

func resolvedPath(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("resolve executable: %v", err)
	}
	return resolved
}

func findTool(t *testing.T, tools []inventory.Tool, id string) inventory.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.ID == id {
			return tool
		}
	}
	t.Fatalf("tool %q not found in %#v", id, tools)
	return inventory.Tool{}
}

func findOutdated(t *testing.T, packages []OutdatedPackage, kind PackageKind, name string) OutdatedPackage {
	t.Helper()
	for _, item := range packages {
		if item.Kind == kind && item.Name == name {
			return item
		}
	}
	t.Fatalf("outdated package %q/%q not found in %#v", kind, name, packages)
	return OutdatedPackage{}
}

func assertReadOnlyCalls(t *testing.T, calls []recordedCall, brewPath, gitPath string) {
	t.Helper()
	want := [][]string{{"--version"}, {"--prefix"}, {"--repository"}, {"--cellar"}, {"--caskroom"}, {"-C", "/opt/homebrew", "remote", "get-url", "origin"}, {"info", "--json=v2", "--installed"}, {"outdated", "--json=v2"}}
	if len(calls) != len(want) {
		t.Fatalf("calls = %d, want %d: %#v", len(calls), len(want), calls)
	}
	for index, call := range calls {
		if !slices.Equal(call.args, want[index]) {
			t.Fatalf("call %d args = %#v, want %#v", index, call.args, want[index])
		}
		wantPath := brewPath
		if index == 5 {
			wantPath = gitPath
		}
		if call.path != wantPath {
			t.Fatalf("call %d path = %q, want %q", index, call.path, wantPath)
		}
		for _, key := range []string{"HOMEBREW_NO_AUTO_UPDATE", "HOMEBREW_NO_ANALYTICS", "HOMEBREW_NO_ENV_HINTS"} {
			if call.env[key] != "1" {
				t.Fatalf("call %d environment %s = %q", index, key, call.env[key])
			}
		}
	}
}

func assertFinding(t *testing.T, findings []inventory.Finding, code string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code {
			return
		}
	}
	t.Fatalf("finding %q not found in %#v", code, findings)
}

func assertSchemaCompatibleTools(t *testing.T, tools []inventory.Tool) {
	t.Helper()
	source := sourceMetadata("fixture system", fixtureTime, inventory.ConfidenceHigh)
	_, err := inventory.Marshal(inventory.Inventory{
		SchemaVersion: inventory.SchemaVersion,
		GeneratedAt:   fixtureTime,
		System: inventory.System{
			OS: inventory.OSMacOS, OSVersion: "fixture", OSBuild: "fixture",
			Architecture: inventory.ArchitectureARM64, ProcessArchitecture: inventory.ArchitectureARM64,
			TranslationState: inventory.TranslationStateNative,
			Shell:            inventory.Shell{LoginPath: "unknown", LoginName: "unknown", InvokingPath: "unknown", InvokingName: "unknown"},
			PathEntries:      []inventory.PathEntry{}, Sources: []inventory.SourceMetadata{source},
		},
		Tools: tools, Findings: []inventory.Finding{},
	})
	if err != nil {
		t.Fatalf("mapped tools violate Inventory Schema: %v", err)
	}
}

func cloneMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
