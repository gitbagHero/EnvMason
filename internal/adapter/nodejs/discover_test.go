package nodejs

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

var fixtureTime = time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)

type runnerCall struct {
	path      string
	args      []string
	directory string
	env       map[string]string
}

type fakeRunner struct {
	versions map[string]string
	failures map[string]error
	calls    []runnerCall
}

func (r *fakeRunner) Run(_ context.Context, path string, args []string, directory string, environment map[string]string) (string, error) {
	r.calls = append(r.calls, runnerCall{path: path, args: append([]string{}, args...), directory: directory, env: cloneMap(environment)})
	if err := r.failures[path]; err != nil {
		return "", err
	}
	if value, ok := r.versions[path]; ok {
		return value, nil
	}
	return "", errors.New("unexpected command")
}

func TestDiscoverNoNode(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	result, err := discover(context.Background(), fixtureRequest(root, []string{root}), dependencies{runner: newFakeRunner()})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.State != StateNotInstalled || result.NVM.State != StateNotInstalled || len(result.Nodes) != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestDiscoverSystemNodeOnly(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	bin := filepath.Join(root, "system", "bin")
	node := writeExecutable(t, bin, "node")
	runner := newFakeRunner()
	runner.versions[node] = "v20.19.4"
	result, err := discover(context.Background(), fixtureRequest(root, []string{bin}), dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].Manager != ManagerSystem || !result.Nodes[0].Effective || result.Nodes[0].Version != "v20.19.4" {
		t.Fatalf("nodes = %#v", result.Nodes)
	}
	assertVersionCallsReadOnly(t, runner.calls)
}

func TestDiscoverNVMVersionsDefaultAndPackageOwnership(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	nvm := filepath.Join(root, ".nvm")
	createNVMVersion(t, nvm, "v22.22.3")
	activeBin := createNVMVersion(t, nvm, "v24.12.0")
	writeAlias(t, nvm, "default", "24")
	npm := writeExecutable(t, activeBin, "npm")
	createCorepackProxy(t, activeBin, "pnpm", "0.35.0")
	runner := newFakeRunner()
	runner.versions[npm] = "11.6.2"
	request := fixtureRequest(root, []string{activeBin})
	request.NVMDirectory = nvm
	request.NVMBin = activeBin

	result, err := discover(context.Background(), request, dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if result.NVM.State != StateInstalled || !result.NVM.Loaded || result.NVM.DefaultAlias != "24" || result.NVM.DefaultVersion != "v24.12.0" {
		t.Fatalf("NVM = %#v", result.NVM)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("nodes = %#v", result.Nodes)
	}
	retained := findNode(t, result.Nodes, "v22.22.3")
	defaultNode := findNode(t, result.Nodes, "v24.12.0")
	if retained.Default || retained.Effective || !defaultNode.Default || !defaultNode.Effective {
		t.Fatalf("retained/default = %#v / %#v", retained, defaultNode)
	}
	npmResult := findPackage(t, result.PackageManagers, "npm", npm)
	if npmResult.Version != "11.6.2" || npmResult.NodeInstallationID != defaultNode.ID || !npmResult.Effective {
		t.Fatalf("npm = %#v", npmResult)
	}
	pnpm := findPackageByName(t, result.PackageManagers, "pnpm")
	if !pnpm.CorepackProxy || pnpm.Version != unknown || pnpm.ProviderVersion != "0.35.0" || pnpm.NodeInstallationID != defaultNode.ID {
		t.Fatalf("pnpm = %#v", pnpm)
	}
	for _, call := range runner.calls {
		if filepath.Base(call.path) == "pnpm" {
			t.Fatal("Corepack pnpm proxy was executed")
		}
	}
	assertVersionCallsReadOnly(t, runner.calls)
}

func TestDiscoverMultipleSourcesAndPATHShadowing(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	nvm := filepath.Join(root, ".nvm")
	nvmBin := createNVMVersion(t, nvm, "v22.22.3")
	writeAlias(t, nvm, "default", "node")
	systemBin := filepath.Join(root, "usr", "bin")
	systemNode := writeExecutable(t, systemBin, "node")
	runner := newFakeRunner()
	runner.versions[systemNode] = "v20.19.4"
	request := fixtureRequest(root, []string{nvmBin, systemBin})
	request.NVMDirectory = nvm
	request.NVMBin = nvmBin

	result, err := discover(context.Background(), request, dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Nodes) != 2 || findNode(t, result.Nodes, "v22.22.3").Manager != ManagerNVM || findNode(t, result.Nodes, "v20.19.4").Manager != ManagerSystem {
		t.Fatalf("nodes = %#v", result.Nodes)
	}
	assertFindingCode(t, result.Findings, "EXECUTABLE_PATH_SHADOWED")
	assertFindingCode(t, result.Findings, "NODE_MULTIPLE_SOURCES")
}

func TestDiscoverNVMNotLoadedStillFindsDiskVersions(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	nvm := filepath.Join(root, ".nvm")
	createNVMVersion(t, nvm, "v18.20.8")
	writeAlias(t, nvm, "default", "18")
	request := fixtureRequest(root, []string{filepath.Join(root, "empty")})

	result, err := discover(context.Background(), request, dependencies{runner: newFakeRunner()})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].Version != "v18.20.8" || result.NVM.Loaded {
		t.Fatalf("result = %#v", result)
	}
	assertFindingCode(t, result.Findings, "NVM_NOT_LOADED")
}

func TestDiscoverHomebrewNode(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	root := t.TempDir()
	prefix := filepath.Join(root, "homebrew")
	bin := filepath.Join(prefix, "bin")
	node := writeExecutable(t, bin, "node")
	runner := newFakeRunner()
	runner.versions[node] = "24.4.0"
	request := fixtureRequest(root, []string{bin})
	request.HomebrewPrefixes = []string{prefix}
	result, err := discover(context.Background(), request, dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(result.Nodes) != 1 || result.Nodes[0].Manager != ManagerHomebrew || result.Nodes[0].Version != "v24.4.0" {
		t.Fatalf("nodes = %#v", result.Nodes)
	}
}

func TestDiscoverFailureDoesNotLeakAndContinues(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	const secret = "node-secret-must-not-leak"
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	node := writeExecutable(t, bin, "node")
	npm := writeExecutable(t, bin, "npm")
	runner := newFakeRunner()
	runner.failures[node] = errors.New(secret)
	runner.versions[npm] = "10.9.4"
	result, err := discover(context.Background(), fixtureRequest(root, []string{bin}), dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), secret) {
		t.Fatal("runner error leaked into result")
	}
	assertFindingCode(t, result.Findings, "NODE_VERSION_QUERY_FAILED")
	if findPackageByName(t, result.PackageManagers, "npm").Version != "10.9.4" {
		t.Fatal("package-manager discovery did not continue")
	}
}

func TestDiscoverRejectsUntrustedVersionOutput(t *testing.T) {
	t.Parallel()
	requirePOSIXFixture(t)

	const secret = "secret value must not leak"
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	node := writeExecutable(t, bin, "node")
	npm := writeExecutable(t, bin, "npm")
	runner := newFakeRunner()
	runner.versions[node] = secret
	runner.versions[npm] = secret
	result, err := discover(context.Background(), fixtureRequest(root, []string{bin}), dependencies{runner: runner})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	data, _ := json.Marshal(result)
	if strings.Contains(string(data), secret) {
		t.Fatal("untrusted version output leaked into result")
	}
	assertFindingCode(t, result.Findings, "NODE_VERSION_OUTPUT_INVALID")
	assertFindingCode(t, result.Findings, "PACKAGE_MANAGER_VERSION_OUTPUT_INVALID")
}

func TestResolveDefaultAliasChainAndLoop(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	writeAlias(t, root, "default", "lts/*")
	writeAlias(t, root, "lts/*", "lts/krypton")
	writeAlias(t, root, "lts/krypton", "v24.12")
	alias, version, err := resolveDefaultAlias(root, []string{"v22.22.3", "v24.12.0"})
	if err != nil || alias != "lts/*" || version != "v24.12.0" {
		t.Fatalf("alias/version = %q/%q, err %v", alias, version, err)
	}
	writeAlias(t, root, "default", "loop/a")
	writeAlias(t, root, "loop/a", "loop/b")
	writeAlias(t, root, "loop/b", "loop/a")
	if _, _, err := resolveDefaultAlias(root, []string{"v24.12.0"}); err == nil {
		t.Fatal("recursive alias unexpectedly resolved")
	}
}

func TestDiscoverRealNodeEnvironmentReadOnly(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("real Node.js acceptance requires macOS")
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	home, _ := os.UserHomeDir()
	nvmDirectory := os.Getenv("NVM_DIR")
	if nvmDirectory == "" {
		nvmDirectory = filepath.Join(home, ".nvm")
	}
	request := Request{
		PathDirectories: strings.Split(os.Getenv("PATH"), string(os.PathListSeparator)), WorkingDirectory: workingDirectory,
		Home: home, NVMDirectory: nvmDirectory, NVMBin: os.Getenv("NVM_BIN"),
		HomebrewPrefixes: []string{"/opt/homebrew", "/usr/local"}, CollectedAt: time.Now().UTC(),
		ProcessArchitecture: inventory.Architecture(runtime.GOARCH),
	}
	before := nvmSnapshot(t, nvmDirectory)
	result, err := Discover(context.Background(), request)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	after := nvmSnapshot(t, nvmDirectory)
	if !slices.Equal(before, after) {
		t.Fatal("NVM version or alias metadata changed during discovery")
	}
	if result.State != StateInstalled || result.CurrentNodeID == "" {
		t.Fatalf("real result = %#v", result)
	}
	if result.NVM.State == StateInstalled && len(result.Nodes) < 1 {
		t.Fatal("installed NVM returned no Node versions")
	}
	t.Logf("real discovery: nodes=%d package_managers=%d findings=%d current=%s default=%s", len(result.Nodes), len(result.PackageManagers), len(result.Findings), result.CurrentNodeID, result.NVM.DefaultVersion)
	for _, finding := range result.Findings {
		t.Logf("finding: %s (%s)", finding.Code, strings.Join(finding.Evidence, ", "))
	}
	for _, manager := range result.PackageManagers {
		if manager.CorepackProxy && manager.Version != unknown {
			t.Fatalf("Corepack proxy reported a concrete dynamic version: %#v", manager)
		}
	}
}

func fixtureRequest(root string, path []string) Request {
	return Request{PathDirectories: path, WorkingDirectory: root, Home: root, CollectedAt: fixtureTime, ProcessArchitecture: inventory.ArchitectureARM64}
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{versions: make(map[string]string), failures: make(map[string]error)}
}

func createNVMVersion(t *testing.T, nvm, version string) string {
	t.Helper()
	bin := filepath.Join(nvm, "versions", "node", version, "bin")
	writeExecutable(t, bin, "node")
	return bin
}

func writeExecutable(t *testing.T, directory, name string) string {
	t.Helper()
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 99\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func writeAlias(t *testing.T, nvm, name, value string) {
	t.Helper()
	path := filepath.Join(nvm, "alias", filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll alias: %v", err)
	}
	if err := os.WriteFile(path, []byte(value+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile alias: %v", err)
	}
}

func createCorepackProxy(t *testing.T, bin, name, version string) string {
	t.Helper()
	root := filepath.Join(filepath.Dir(bin), "lib", "node_modules", "corepack")
	target := filepath.Join(root, "dist", name+".js")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll corepack: %v", err)
	}
	if err := os.WriteFile(target, []byte("#!/usr/bin/env node\n"), 0o755); err != nil {
		t.Fatalf("WriteFile corepack target: %v", err)
	}
	data := []byte(`{"name":"corepack","version":"` + version + `"}`)
	if err := os.WriteFile(filepath.Join(root, "package.json"), data, 0o644); err != nil {
		t.Fatalf("WriteFile package.json: %v", err)
	}
	link := filepath.Join(bin, name)
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	return link
}

func findNode(t *testing.T, nodes []NodeInstallation, version string) NodeInstallation {
	t.Helper()
	for _, node := range nodes {
		if node.Version == version {
			return node
		}
	}
	t.Fatalf("Node %q not found in %#v", version, nodes)
	return NodeInstallation{}
}

func findPackage(t *testing.T, managers []PackageManager, name, path string) PackageManager {
	t.Helper()
	for _, manager := range managers {
		if manager.Name == name && filepath.Base(manager.Path) == filepath.Base(path) {
			return manager
		}
	}
	t.Fatalf("package manager %q at %q not found in %#v", name, path, managers)
	return PackageManager{}
}

func findPackageByName(t *testing.T, managers []PackageManager, name string) PackageManager {
	t.Helper()
	for _, manager := range managers {
		if manager.Name == name {
			return manager
		}
	}
	t.Fatalf("package manager %q not found in %#v", name, managers)
	return PackageManager{}
}

func assertFindingCode(t *testing.T, findings []inventory.Finding, code string) {
	t.Helper()
	for _, finding := range findings {
		if finding.Code == code {
			return
		}
	}
	t.Fatalf("finding %q not found in %#v", code, findings)
}

func assertVersionCallsReadOnly(t *testing.T, calls []runnerCall) {
	t.Helper()
	for _, call := range calls {
		if !slices.Equal(call.args, []string{"--version"}) {
			t.Fatalf("unsafe args = %#v", call.args)
		}
		for _, key := range []string{"COREPACK_ENABLE_NETWORK", "COREPACK_DEFAULT_TO_LATEST", "COREPACK_ENABLE_AUTO_PIN", "COREPACK_ENABLE_PROJECT_SPEC", "COREPACK_ENABLE_DOWNLOAD_PROMPT"} {
			if call.env[key] != "0" {
				t.Fatalf("%s = %q", key, call.env[key])
			}
		}
	}
}

func cloneMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func requirePOSIXFixture(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("Node/NVM executable fixture uses POSIX execute permissions and symlinks")
	}
}

func nvmSnapshot(t *testing.T, directory string) []string {
	t.Helper()
	result := []string{}
	for _, relative := range []string{"alias", filepath.Join("versions", "node")} {
		root := filepath.Join(directory, relative)
		_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			info, infoErr := entry.Info()
			if infoErr == nil {
				rel, _ := filepath.Rel(directory, path)
				result = append(result, rel+"|"+info.ModTime().UTC().Format(time.RFC3339Nano)+"|"+info.Mode().String())
			}
			return nil
		})
	}
	slices.Sort(result)
	return result
}
