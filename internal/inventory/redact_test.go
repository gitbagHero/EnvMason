package inventory

import (
	"path/filepath"
	"testing"
)

func TestRedactInstallationPathsUsesLongestPrivateRootWithoutMutatingInput(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	nvmDirectory := filepath.Join(home, "custom-nvm")
	node := filepath.Join(nvmDirectory, "versions", "node", "v24.14.0", "bin", "node")
	value := Inventory{Tools: []Tool{{Installations: []Installation{{Path: node}, {Path: "/opt/homebrew/bin/node"}}}}}

	result, err := RedactInstallationPaths(value,
		PathRedaction{Root: home, Placeholder: "$HOME"},
		PathRedaction{Root: nvmDirectory, Placeholder: "$NVM_DIR"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tools[0].Installations[0].Path != "$NVM_DIR/versions/node/v24.14.0/bin/node" ||
		result.Tools[0].Installations[1].Path != "/opt/homebrew/bin/node" {
		t.Fatalf("redacted paths = %#v", result.Tools[0].Installations)
	}
	if value.Tools[0].Installations[0].Path != node {
		t.Fatal("input Inventory was mutated")
	}
}

func TestRedactInstallationPathsRejectsRelativeRootAndUnsafePlaceholder(t *testing.T) {
	t.Parallel()
	for _, redaction := range []PathRedaction{
		{Root: "relative", Placeholder: "$HOME"},
		{Root: t.TempDir(), Placeholder: "$HOME/private"},
	} {
		if _, err := RedactInstallationPaths(Inventory{}, redaction); err == nil {
			t.Fatalf("redaction %#v was accepted", redaction)
		}
	}
}
