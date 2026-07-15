package inventory

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateJSONAcceptsFixturesAndPublicExample(t *testing.T) {
	t.Parallel()

	paths := []string{
		filepath.Join("testdata", "valid", "minimal.json"),
		filepath.Join("..", "..", "examples", "inventory-report.json"),
	}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if err := ValidateJSON(readFile(t, path)); err != nil {
				t.Fatalf("ValidateJSON(%q) returned an error: %v", path, err)
			}
		})
	}
}

func TestValidateJSONRejectsInvalidFixtures(t *testing.T) {
	t.Parallel()

	paths, err := filepath.Glob(filepath.Join("testdata", "invalid", "*.json"))
	if err != nil {
		t.Fatalf("glob invalid fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Fatal("no invalid fixtures found")
	}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if err := ValidateJSON(readFile(t, path)); err == nil {
				t.Fatalf("ValidateJSON(%q) unexpectedly succeeded", path)
			}
		})
	}
}

func TestDecodeRejectsMalformedAndTrailingJSON(t *testing.T) {
	t.Parallel()

	for name, data := range map[string][]byte{
		"malformed": []byte(`{"schema_version":`),
		"trailing":  append(readFile(t, filepath.Join("testdata", "valid", "minimal.json")), []byte(` {}`)...),
	} {
		name, data := name, data
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := Decode(data); err == nil {
				t.Fatal("Decode unexpectedly succeeded")
			}
		})
	}
}

func TestDecodePublicExample(t *testing.T) {
	t.Parallel()

	value, err := Decode(readFile(t, filepath.Join("..", "..", "examples", "inventory-report.json")))
	if err != nil {
		t.Fatalf("Decode public example: %v", err)
	}
	if value.SchemaVersion != SchemaVersion {
		t.Fatalf("schema version = %q, want %q", value.SchemaVersion, SchemaVersion)
	}
	if len(value.Tools) != 1 || len(value.Tools[0].Installations) != 2 {
		t.Fatalf("public example tool/installations count = %d/%d, want 1/2", len(value.Tools), len(value.Tools[0].Installations))
	}
}

func TestMarshalIsDeterministicAndMatchesGolden(t *testing.T) {
	t.Parallel()

	value := fixtureInventory()
	first, err := Marshal(value)
	if err != nil {
		t.Fatalf("first Marshal: %v", err)
	}
	second, err := Marshal(value)
	if err != nil {
		t.Fatalf("second Marshal: %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Marshal output differs for identical input")
	}

	want := readFile(t, filepath.Join("testdata", "golden", "inventory.json"))
	if !bytes.Equal(first, want) {
		t.Fatalf("Marshal output differs from golden file\n--- got ---\n%s\n--- want ---\n%s", first, want)
	}
}

func TestMarshalDecodeRoundTrip(t *testing.T) {
	t.Parallel()

	want := fixtureInventory()
	data, err := Marshal(want)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	dataAfterRoundTrip, err := Marshal(got)
	if err != nil {
		t.Fatalf("Marshal after round trip: %v", err)
	}
	if !bytes.Equal(data, dataAfterRoundTrip) {
		t.Fatal("encoded inventory changed after round trip")
	}
}

func fixtureInventory() Inventory {
	collectedAt := time.Date(2026, 7, 15, 0, 0, 0, 0, time.UTC)
	source := SourceMetadata{
		Kind:        SourceFixture,
		Name:        "deterministic golden fixture",
		CollectedAt: collectedAt,
		Confidence:  ConfidenceHigh,
	}
	return Inventory{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   collectedAt,
		System: System{
			OS:                  OSMacOS,
			OSVersion:           "15.5",
			Architecture:        ArchitectureARM64,
			ProcessArchitecture: ArchitectureARM64,
			Sources:             []SourceMetadata{source},
		},
		Tools: []Tool{
			{
				ID:          "runtime.node",
				DisplayName: "Node.js",
				Category:    CategoryRuntime,
				Installations: []Installation{
					{
						ID:                "node-homebrew-24",
						Version:           "v24.4.0",
						NormalizedVersion: "24.4.0",
						Path:              "/opt/homebrew/bin/node",
						Architecture:      ArchitectureARM64,
						Manager:           "homebrew",
						ActiveState:       ActiveStateActive,
						DefaultState:      DefaultStateDefault,
						InstallReason:     InstallReasonDirect,
						Sources:           []SourceMetadata{source},
					},
				},
			},
		},
		Findings: []Finding{},
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return data
}
