package nodetools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestInspectBindsToolsToTargetNVMNode(t *testing.T) {
	directory := nodeToolsFixture(t)
	value, err := Inspect(directory, "24.12.0", "v26.5.0")
	if err != nil {
		t.Fatal(err)
	}
	if value.NodeVersion != "24.12.0" || value.NPM.Version != "11.6.2" || value.NPM.Provider != plan.NodeToolProviderNPM {
		t.Fatalf("baseline = %#v", value)
	}
	if value.Corepack.Version != "0.34.5" || value.Corepack.Provider != plan.NodeToolProviderNPM {
		t.Fatalf("corepack = %#v", value.Corepack)
	}
	if value.PNPM.Version != "unknown" || value.PNPM.Provider != plan.NodeToolProviderCorepack ||
		value.PNPM.ControlDigest != value.Corepack.ControlDigest {
		t.Fatalf("pnpm = %#v", value.PNPM)
	}
}

func TestDefinitionsBuildFixedStructuredCommands(t *testing.T) {
	baseline, err := Inspect(nodeToolsFixture(t), "24.12.0", "v26.5.0")
	if err != nil {
		t.Fatal(err)
	}
	options := Options{
		Baseline: baseline, Targets: Targets{NPM: "12.0.1", PNPM: "11.1.0"},
		Home: t.TempDir(), Temporary: t.TempDir(), ProxyValues: map[string]string{"HTTPS_PROXY": "https://secret.invalid"},
	}
	npm := findDefinition(t, Definitions(options), plan.NodeToolNPM, plan.NodeToolProviderNPM)
	spec, err := npm.Build(plan.Action{ToolID: plan.NodeToolNPM, Operation: "update_version", Adapter: plan.NodeToolProviderNPM, TargetVersion: "12.0.1"})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Executable != baseline.NPM.Executable || strings.Contains(strings.Join(spec.Args, " "), "latest") {
		t.Fatalf("npm spec = %#v", spec)
	}
	arguments := strings.Join(spec.Args, "\n")
	for _, expected := range []string{"install", "--global", "--ignore-scripts", "--registry=" + RegistryURL, "npm@12.0.1"} {
		if !strings.Contains(arguments, expected) {
			t.Fatalf("arguments %q do not contain %q", arguments, expected)
		}
	}
	environment := strings.Join(spec.Environment, "\n")
	for _, expected := range []string{"NPM_CONFIG_USERCONFIG=/dev/null", "COREPACK_ENABLE_PROJECT_SPEC=0", "COREPACK_DEFAULT_TO_LATEST=0"} {
		if !strings.Contains(environment, expected) {
			t.Fatalf("environment does not contain %q: %s", expected, environment)
		}
	}

	pnpm := findDefinition(t, Definitions(options), plan.NodeToolPNPM, plan.NodeToolProviderCorepack)
	pnpmSpec, err := pnpm.Build(plan.Action{ToolID: plan.NodeToolPNPM, Operation: "update_version", Adapter: plan.NodeToolProviderCorepack, TargetVersion: "11.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if pnpmSpec.Executable != baseline.Corepack.Executable ||
		strings.Join(pnpmSpec.Args, " ") != "install --global pnpm@11.1.0" {
		t.Fatalf("pnpm spec = %#v", pnpmSpec)
	}
}

func TestDefinitionVerifiesCorepackProxyAndNodeVersion(t *testing.T) {
	baseline, err := Inspect(nodeToolsFixture(t), "24.12.0", "v26.5.0")
	if err != nil {
		t.Fatal(err)
	}
	runner := versionRunner{versions: map[string]string{
		baseline.PNPM.Executable: "11.1.0\n",
		baseline.NodeBinary:      "v24.12.0\n",
	}}
	options := Options{
		Baseline: baseline, Targets: Targets{PNPM: "11.1.0"},
		Home: t.TempDir(), Temporary: t.TempDir(), Verifier: runner,
	}
	definition := findDefinition(t, Definitions(options), plan.NodeToolPNPM, plan.NodeToolProviderCorepack)
	action := plan.Action{ToolID: plan.NodeToolPNPM, Operation: "update_version", Adapter: plan.NodeToolProviderCorepack, TargetVersion: "11.1.0"}
	satisfied, err := definition.Satisfied(context.Background(), action)
	if err != nil || !satisfied {
		t.Fatalf("satisfied = %v, %v", satisfied, err)
	}
	if err := definition.Verify(context.Background(), action, execution.ProcessResult{}); err != nil {
		t.Fatal(err)
	}
}

func TestInspectAndBuildIndependentPNPMThroughTargetNPM(t *testing.T) {
	directory := nodeToolsFixture(t)
	root := filepath.Join(directory, "versions", "node", "v24.12.0")
	if err := os.Remove(filepath.Join(root, "bin", "pnpm")); err != nil {
		t.Fatal(err)
	}
	writePackage(t, root, "pnpm", "10.0.0", "bin/pnpm.cjs")
	baseline, err := Inspect(directory, "24.12.0", "v26.5.0")
	if err != nil {
		t.Fatal(err)
	}
	if baseline.PNPM.Provider != plan.NodeToolProviderNPM || baseline.PNPM.Version != "10.0.0" {
		t.Fatalf("pnpm = %#v", baseline.PNPM)
	}
	options := Options{
		Baseline: baseline, Targets: Targets{PNPM: "11.1.0"},
		Home: t.TempDir(), Temporary: t.TempDir(),
	}
	definition := findDefinition(t, Definitions(options), plan.NodeToolPNPM, plan.NodeToolProviderNPM)
	spec, err := definition.Build(plan.Action{
		ToolID: plan.NodeToolPNPM, Operation: "update_version",
		Adapter: plan.NodeToolProviderNPM, TargetVersion: "11.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Executable != baseline.NPM.Executable ||
		!strings.Contains(strings.Join(spec.Args, " "), "pnpm@11.1.0") {
		t.Fatalf("spec = %#v", spec)
	}
}

func TestInspectRejectsToolThatEscapesNodeRoot(t *testing.T) {
	directory := nodeToolsFixture(t)
	root := filepath.Join(directory, "versions", "node", "v24.12.0")
	if err := os.Remove(filepath.Join(root, "bin", "npm")); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "npm")
	if err := os.WriteFile(outside, []byte("outside"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "bin", "npm")); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(directory, "24.12.0", "v26.5.0"); err == nil || !strings.Contains(err.Error(), "outside") {
		t.Fatalf("error = %v", err)
	}
}

type versionRunner struct {
	versions map[string]string
}

func (runner versionRunner) Run(_ context.Context, spec execution.CommandSpec) execution.ProcessResult {
	code := 0
	value, ok := runner.versions[spec.Executable]
	if !ok {
		code = 1
	}
	return execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: value}}
}

func findDefinition(t *testing.T, definitions []execution.Definition, toolID, provider string) execution.Definition {
	t.Helper()
	for _, definition := range definitions {
		if definition.Key.ToolID == toolID && definition.Key.Adapter == provider {
			return definition
		}
	}
	t.Fatalf("definition %s/%s not found", toolID, provider)
	return execution.Definition{}
}

func nodeToolsFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	writeFixture(t, filepath.Join(directory, "nvm.sh"), "fixture", 0o644)
	writeFixture(t, filepath.Join(directory, "alias", "default"), "v24.12.0\n", 0o644)
	root := filepath.Join(directory, "versions", "node", "v24.12.0")
	writeFixture(t, filepath.Join(root, "bin", "node"), "node", 0o755)
	writePackage(t, root, "npm", "11.6.2", "bin/npm-cli.js")
	writePackage(t, root, "corepack", "0.34.5", "dist/corepack.js")
	if err := os.Symlink("../lib/node_modules/corepack/dist/pnpm.js", filepath.Join(root, "bin", "pnpm")); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(root, "lib", "node_modules", "corepack", "dist", "pnpm.js"), "pnpm", 0o644)
	return directory
}

func writePackage(t *testing.T, root, name, version, entry string) {
	t.Helper()
	packageRoot := filepath.Join(root, "lib", "node_modules", name)
	writeFixture(t, filepath.Join(packageRoot, "package.json"), `{"name":"`+name+`","version":"`+version+`"}`, 0o644)
	writeFixture(t, filepath.Join(packageRoot, filepath.FromSlash(entry)), name, 0o644)
	if err := os.MkdirAll(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "lib", "node_modules", name, filepath.FromSlash(entry)), filepath.Join(root, "bin", name)); err != nil {
		t.Fatal(err)
	}
}

func writeFixture(t *testing.T, path, value string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(value), mode); err != nil {
		t.Fatal(err)
	}
}
