package nvm

import (
	"os"
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

func proxyValuesFromEnvironment() map[string]string {
	result := map[string]string{}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "all_proxy", "no_proxy"} {
		if value := os.Getenv(key); value != "" {
			result[key] = value
		}
	}
	return result
}
