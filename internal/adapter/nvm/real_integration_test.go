package nvm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

// TestRealNVMInstall is opt-in because it downloads a real Node.js binary.
// Run it only in a disposable VM/container with ENVMASON_REAL_NVM_DIR set.
func TestRealNVMInstall(t *testing.T) {
	directory := os.Getenv("ENVMASON_REAL_NVM_DIR")
	if directory == "" {
		t.Skip("set ENVMASON_REAL_NVM_DIR in a disposable environment")
	}
	target := os.Getenv("ENVMASON_REAL_NVM_TARGET")
	if target == "" {
		t.Fatal("ENVMASON_REAL_NVM_TARGET is required")
	}
	active := os.Getenv("ENVMASON_REAL_NVM_ACTIVE")
	if active == "" {
		t.Fatal("ENVMASON_REAL_NVM_ACTIVE is required")
	}
	baseline, err := Inspect(directory, active)
	if err != nil {
		t.Fatal(err)
	}
	activeBinary, err := targetBinary(directory, active)
	if err != nil {
		t.Fatal(err)
	}
	definition := Definition(Options{Baseline: baseline, ActiveBinary: activeBinary, Home: os.Getenv("HOME"), Temporary: os.Getenv("TMPDIR"), ProxyValues: proxyValuesFromEnvironment()})
	action := plan.Action{ToolID: "runtime.node", Operation: "install_version", Adapter: "nvm", TargetVersion: target, Risk: plan.RiskR2}
	if err := definition.Preflight(t.Context(), action); err != nil {
		t.Fatal(err)
	}
	satisfied, err := definition.Satisfied(t.Context(), action)
	if err != nil {
		t.Fatal(err)
	}
	if !satisfied {
		spec, err := definition.Build(action)
		if err != nil {
			t.Fatal(err)
		}
		result := (execution.OSRunner{}).Run(t.Context(), spec)
		if result.Failure != nil {
			t.Fatalf("real NVM install: %v\nstdout: %s\nstderr: %s", result.Failure, result.Stdout.Text, result.Stderr.Text)
		}
		if err := definition.Verify(t.Context(), action, result); err != nil {
			t.Fatal(err)
		}
	}
	second, err := definition.Satisfied(t.Context(), action)
	if err != nil || !second {
		t.Fatalf("real NVM second-run satisfaction = %t, %v", second, err)
	}
	current, err := Inspect(directory, active)
	if err != nil {
		t.Fatal(err)
	}
	if current.DefaultAliasDigest != baseline.DefaultAliasDigest {
		t.Fatal("real NVM install changed the default alias")
	}
	original := map[string]bool{}
	for _, version := range current.InstalledVersions {
		original[version] = true
	}
	for _, version := range baseline.InstalledVersions {
		if !original[version] {
			t.Fatalf("real NVM install removed %s", version)
		}
	}
}

// TestRealNVMDefaultSetRestore uses a disposable NVM directory and a caller-
// supplied real nvm.sh. It never changes the source NVM installation.
func TestRealNVMDefaultSetRestore(t *testing.T) {
	source := os.Getenv("ENVMASON_REAL_NVM_SCRIPT")
	if source == "" {
		t.Skip("set ENVMASON_REAL_NVM_SCRIPT to a real nvm.sh")
	}
	script, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "nvm.sh"), script, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(directory, "alias"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "alias", "default"), []byte("v22.0.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"22.0.0", "24.14.0"} {
		bin := filepath.Join(directory, "versions", "node", "v"+version, "bin")
		if err := os.MkdirAll(bin, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(bin, "node"), []byte("#!/bin/sh\nprintf 'v"+version+"\\n'\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	baseline, err := InspectDefault(directory, "v22.0.0")
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Baseline: baseline, ActiveBinary: filepath.Join(directory, "versions", "node", "v22.0.0", "bin", "node"), Home: t.TempDir(), Temporary: t.TempDir()}
	set := SetDefaultDefinition(DefaultOptions{Options: options, DesiredAlias: "v24.14.0", DesiredVersion: "24.14.0"})
	action := plan.Action{ToolID: "runtime.node", Operation: "set_default", Adapter: "nvm", TargetVersion: "24.14.0", Risk: plan.RiskR3}
	spec, err := set.Build(action)
	if err != nil {
		t.Fatal(err)
	}
	result := (execution.OSRunner{}).Run(t.Context(), spec)
	if result.Failure != nil {
		t.Fatalf("real nvm.sh set default: %v\n%s", result.Failure, result.Stderr.Text)
	}
	if err := set.Verify(t.Context(), action, result); err != nil {
		t.Fatal(err)
	}
	changed, err := InspectDefault(directory, "v22.0.0")
	if err != nil || changed.DefaultAlias != "v24.14.0" {
		t.Fatalf("changed alias = %#v, %v", changed, err)
	}
	restoreOptions := options
	restoreOptions.Baseline = changed
	restore := RestoreDefaultDefinition(DefaultOptions{Options: restoreOptions, DesiredAlias: baseline.DefaultAlias, DesiredVersion: strings.TrimPrefix(baseline.DefaultVersion, "v")})
	restoreAction := plan.Action{ToolID: "runtime.node", Operation: "restore_default", Adapter: "nvm", TargetVersion: strings.TrimPrefix(baseline.DefaultVersion, "v"), Risk: plan.RiskR3}
	restoreSpec, err := restore.Build(restoreAction)
	if err != nil {
		t.Fatal(err)
	}
	restoreResult := (execution.OSRunner{}).Run(t.Context(), restoreSpec)
	if restoreResult.Failure != nil {
		t.Fatalf("real nvm.sh restore default: %v\n%s", restoreResult.Failure, restoreResult.Stderr.Text)
	}
	if err := restore.Verify(t.Context(), restoreAction, restoreResult); err != nil {
		t.Fatal(err)
	}
	final, err := InspectDefault(directory, "v22.0.0")
	if err != nil || final.DefaultAlias != baseline.DefaultAlias || final.DefaultAliasDigest != baseline.DefaultAliasDigest {
		t.Fatalf("restored alias = %#v, %v", final, err)
	}
}

func proxyValuesFromEnvironment() map[string]string {
	result := map[string]string{}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		if value := os.Getenv(key); value != "" {
			result[key] = value
		}
	}
	return result
}
