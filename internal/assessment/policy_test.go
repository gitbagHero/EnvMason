package assessment

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodePolicyStrictValidationAndDefaults(t *testing.T) {
	t.Parallel()
	policy, err := DecodePolicy([]byte(`{"schema_version":"0.1.0","tools":{"runtime.node":{"pin":"22.22.0","ignore_updates":true}}}`))
	if err != nil {
		t.Fatal(err)
	}
	if policy.Tools["runtime.node"].Channel != ChannelLTS || !policy.Tools["runtime.node"].IgnoreUpdates || policy.Tools["runtime.java"].Channel != ChannelLTS {
		t.Fatalf("defaults = %#v", policy)
	}

	tests := []string{
		`{"schema_version":"0.2.0","tools":{}}`,
		`{"schema_version":"0.1.0","tools":{"runtime.python":{}}}`,
		`{"schema_version":"0.1.0","tools":{"runtime.node":{"channel":"nightly"}}}`,
		`{"schema_version":"0.1.0","tools":{"runtime.node":{"pin":"22"}}}`,
		`{"schema_version":"0.1.0","tools":{},"unknown":true}`,
		`{"schema_version":"0.1.0","tools":{}} {}`,
	}
	for _, data := range tests {
		if _, err := DecodePolicy([]byte(data)); err == nil {
			t.Errorf("invalid policy accepted: %s", data)
		}
	}
}

func TestLoadPolicyIsExplicitAndBounded(t *testing.T) {
	t.Parallel()
	if policy, err := LoadPolicy(""); err != nil || policy.Tools["runtime.node"].Channel != ChannelLTS {
		t.Fatalf("default policy = %#v, %v", policy, err)
	}
	directory := t.TempDir()
	path := filepath.Join(directory, "policy.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":"0.1.0","tools":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(path); err != nil {
		t.Fatal(err)
	}
	tooLarge := filepath.Join(directory, "large.json")
	if err := os.WriteFile(tooLarge, []byte(strings.Repeat("x", MaxPolicyBytes+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPolicy(tooLarge); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized policy error = %v", err)
	}
}
