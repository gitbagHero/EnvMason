package nvm

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

func TestDefinitionUsesOnlyFixedShellContract(t *testing.T) {
	t.Parallel()
	baseline := fixtureNVM(t, false)
	definition := Definition(Options{Baseline: baseline, Home: t.TempDir(), ProxyValues: map[string]string{"HTTPS_PROXY": "http://token@example.test"}})
	action := plan.Action{ToolID: "runtime.node", Operation: "install_version", Adapter: "nvm", TargetVersion: "24.14.0", Risk: plan.RiskR2}
	spec, err := definition.Build(action)
	if err != nil {
		t.Fatal(err)
	}
	if spec.Executable != "/bin/bash" || !spec.TerminateTree || spec.Timeout != ActionTimeout || len(spec.Args) != 7 {
		t.Fatalf("spec = %#v", spec)
	}
	if spec.Args[0] != "--noprofile" || spec.Args[1] != "--norc" || spec.Args[2] != "-c" || spec.Args[3] != fixedScript || spec.Args[6] != "24.14.0" {
		t.Fatalf("fixed arguments = %#v", spec.Args)
	}
	joined := strings.Join(spec.Environment, "\n")
	for _, forbidden := range []string{"NVM_NODEJS_ORG_MIRROR", "NPM_TOKEN", "BASH_ENV", "NODE_OPTIONS"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("unsafe environment inherited: %s", joined)
		}
	}
	foundProxy := false
	for _, value := range spec.SensitiveValues {
		foundProxy = foundProxy || value == "http://token@example.test"
	}
	if !foundProxy {
		t.Fatalf("proxy was not marked sensitive: %#v", spec.SensitiveValues)
	}
	malicious := action
	malicious.TargetVersion = "24.14.0; touch /tmp/injected"
	if _, err := definition.Build(malicious); err == nil {
		t.Fatal("command-like target was accepted")
	}
}

func TestFixtureInstallRetainsExistingVersionAndDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("NVM shell fixture requires a POSIX host")
	}
	t.Parallel()
	baseline := fixtureNVM(t, false)
	definition := Definition(Options{Baseline: baseline, Home: t.TempDir()})
	action := plan.Action{ToolID: "runtime.node", Operation: "install_version", Adapter: "nvm", TargetVersion: "24.14.0", Risk: plan.RiskR2}
	before, err := definition.Capture(t.Context(), action)
	if err != nil {
		t.Fatal(err)
	}
	if err := definition.Preflight(t.Context(), action); err != nil {
		t.Fatal(err)
	}
	spec, err := definition.Build(action)
	if err != nil {
		t.Fatal(err)
	}
	result := (execution.OSRunner{}).Run(context.Background(), spec)
	if result.Failure != nil {
		t.Fatalf("fixture install failed: %v / %s", result.Failure, result.Stderr.Text)
	}
	if err := definition.Verify(t.Context(), action, result); err != nil {
		t.Fatal(err)
	}
	after, err := definition.Capture(t.Context(), action)
	if err != nil {
		t.Fatal(err)
	}
	if before.Facts["default_alias_hash"] != after.Facts["default_alias_hash"] || after.Facts["target_installed"] != "true" {
		t.Fatalf("snapshots = %#v / %#v", before, after)
	}
	if _, err := os.Stat(filepath.Join(baseline.Directory, "versions", "node", "v22.0.0", "bin", "node")); err != nil {
		t.Fatal("original Node.js version was removed")
	}
	satisfied, err := definition.Satisfied(t.Context(), action)
	if err != nil || !satisfied {
		t.Fatalf("second-run satisfaction = %t, %v", satisfied, err)
	}
}

func TestFixtureDownloadFailureIsNonZeroAndLeavesDefault(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("NVM shell fixture requires a POSIX host")
	}
	t.Parallel()
	baseline := fixtureNVM(t, true)
	definition := Definition(Options{Baseline: baseline, Home: t.TempDir()})
	action := plan.Action{ToolID: "runtime.node", Operation: "install_version", Adapter: "nvm", TargetVersion: "24.14.0", Risk: plan.RiskR2}
	spec, err := definition.Build(action)
	if err != nil {
		t.Fatal(err)
	}
	result := (execution.OSRunner{}).Run(t.Context(), spec)
	if result.Failure == nil || result.Failure.Code != execution.CodeExitNonZero {
		t.Fatalf("failure = %#v", result)
	}
	current, err := Inspect(baseline.Directory, baseline.ActiveVersion)
	if err != nil || current.DefaultAliasDigest != baseline.DefaultAliasDigest {
		t.Fatalf("default alias changed: %#v, %v", current, err)
	}
}

func TestPartialTargetDirectoryIsNotTreatedAsInstalled(t *testing.T) {
	t.Parallel()
	baseline := fixtureNVM(t, false)
	if err := os.MkdirAll(filepath.Join(baseline.Directory, "versions", "node", "v24.14.0", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	definition := Definition(Options{Baseline: baseline, Home: t.TempDir()})
	action := plan.Action{ToolID: "runtime.node", Operation: "install_version", Adapter: "nvm", TargetVersion: "24.14.0", Risk: plan.RiskR2}
	satisfied, err := definition.Satisfied(t.Context(), action)
	if err != nil || satisfied {
		t.Fatalf("partial target satisfaction = %t, %v", satisfied, err)
	}
	snapshot, err := definition.Capture(t.Context(), action)
	if err != nil || snapshot.Facts["target_installed"] != "false" {
		t.Fatalf("partial target snapshot = %#v, %v", snapshot, err)
	}
}

func TestInspectRejectsSymlinkedScriptAndDefaultAlias(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not generally available to unprivileged Windows CI")
	}
	t.Parallel()
	for _, name := range []string{"nvm.sh", filepath.Join("alias", "default")} {
		t.Run(name, func(t *testing.T) {
			baseline := fixtureNVM(t, false)
			path := filepath.Join(baseline.Directory, name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			target := path + ".real"
			if err := os.WriteFile(target, data, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(path); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			if _, err := Inspect(baseline.Directory, baseline.ActiveVersion); err == nil {
				t.Fatal("symlink was accepted")
			}
		})
	}
}

func TestInspectBoundsScriptAndAliasFiles(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name string
		path string
		size int
	}{
		{name: "script", path: "nvm.sh", size: maximumScript + 1},
		{name: "alias", path: filepath.Join("alias", "default"), size: maximumAlias + 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			baseline := fixtureNVM(t, false)
			if err := os.WriteFile(filepath.Join(baseline.Directory, test.path), []byte(strings.Repeat("x", test.size)), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Inspect(baseline.Directory, baseline.ActiveVersion); err == nil {
				t.Fatal("oversized NVM control file was accepted")
			}
		})
	}
}

func fixtureNVM(t *testing.T, fail bool) Baseline {
	t.Helper()
	directory := t.TempDir()
	if err := os.MkdirAll(filepath.Join(directory, "alias"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	createFixtureNode(t, directory, "22.0.0")
	body := `nvm() {
  if [ "$1" != "install" ]; then return 64; fi
  shift
  target=""
  for argument in "$@"; do
    case "$argument" in -*) ;; *) target="$argument" ;; esac
  done
  if [ -f "$NVM_DIR/fail" ]; then return 42; fi
  destination="$NVM_DIR/versions/node/v$target/bin"
  mkdir -p "$destination"
  printf '#!/bin/sh\nprintf "v%s\\n"\n' "$target" > "$destination/node"
  chmod +x "$destination/node"
}
`
	if err := os.WriteFile(filepath.Join(directory, "nvm.sh"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if fail {
		if err := os.WriteFile(filepath.Join(directory, "fail"), []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	baseline, err := Inspect(directory, "v22.0.0")
	if err != nil {
		t.Fatal(err)
	}
	return baseline
}

func createFixtureNode(t *testing.T, directory, version string) {
	t.Helper()
	bin := filepath.Join(directory, "versions", "node", "v"+version, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\nprintf 'v"+version+"\\n'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}
