package profile

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestYAMLAndJSONRoundTripNormalizedProfile(t *testing.T) {
	yamlInput := []byte(`schema_version: 0.1.0
name: round-trip
modules:
  - id: frontend_node
    variant: minimal
    version_policy:
      strategy: exact
      pin: 22.14.0
    options:
      pnpm: true
  - id: base
`)
	value, err := DecodeYAML(yamlInput)
	if err != nil {
		t.Fatalf("DecodeYAML() error = %v", err)
	}
	if !IsNormalized(value) || value.Modules[0].ID != ModuleBase ||
		value.Modules[1].VersionPolicy.Pin != "22.14.0" {
		t.Fatalf("DecodeYAML() = %#v", value)
	}

	yamlData, err := MarshalYAML(value)
	if err != nil {
		t.Fatalf("MarshalYAML() error = %v", err)
	}
	yamlAgain, err := DecodeYAML(yamlData)
	if err != nil || !reflect.DeepEqual(yamlAgain, value) {
		t.Fatalf("YAML round trip = %#v, %v", yamlAgain, err)
	}

	jsonData, err := MarshalJSON(value)
	if err != nil {
		t.Fatalf("MarshalJSON() error = %v", err)
	}
	if !bytes.HasSuffix(jsonData, []byte("\n")) ||
		!bytes.Contains(jsonData, []byte(`"browser_testing": false`)) {
		t.Fatalf("normalized JSON = %s", jsonData)
	}
	jsonAgain, err := DecodeJSON(jsonData)
	if err != nil || !reflect.DeepEqual(jsonAgain, value) {
		t.Fatalf("JSON round trip = %#v, %v", jsonAgain, err)
	}
}

func TestDecodersRejectUntrustedOrAmbiguousInput(t *testing.T) {
	jsonCases := map[string]struct {
		document string
		message  string
	}{
		"missing version": {`{"name":"test","modules":[{"id":"base"}]}`, "schema_version is required"},
		"old version":     {`{"schema_version":"0.0.1","name":"test","modules":[{"id":"base"}]}`, "migration is unavailable"},
		"root command":    {`{"schema_version":"0.1.0","name":"test","command":"whoami","modules":[{"id":"base"}]}`, "unknown field"},
		"module shell":    {`{"schema_version":"0.1.0","name":"test","modules":[{"id":"base","shell":"sh"}]}`, "unknown field"},
		"option args":     {`{"schema_version":"0.1.0","name":"test","modules":[{"id":"base","options":{"args":["-c"]}}]}`, "unknown field"},
		"trailing value":  {`{"schema_version":"0.1.0","name":"test","modules":[{"id":"base"}]} {}`, "invalid character"},
	}
	for name, test := range jsonCases {
		t.Run("json "+name, func(t *testing.T) {
			_, err := DecodeJSON([]byte(test.document))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("DecodeJSON() error = %v, want containing %q", err, test.message)
			}
		})
	}

	yamlCases := map[string]struct {
		document string
		message  string
	}{
		"unknown command":    {"schema_version: 0.1.0\nname: test\ncommand: whoami\nmodules:\n  - id: base\n", "field command not found"},
		"duplicate key":      {"schema_version: 0.1.0\nname: first\nname: second\nmodules:\n  - id: base\n", "duplicate key"},
		"alias":              {"schema_version: 0.1.0\nname: test\nmodules: &modules\n  - id: base\ncopy: *modules\n", "aliases and anchors"},
		"explicit tag":       {"schema_version: 0.1.0\nname: !!str test\nmodules:\n  - id: base\n", "explicit tags"},
		"multiple documents": {"schema_version: 0.1.0\nname: test\nmodules:\n  - id: base\n---\nschema_version: 0.1.0\nname: second\nmodules:\n  - id: base\n", "multiple documents"},
	}
	for name, test := range yamlCases {
		t.Run("yaml "+name, func(t *testing.T) {
			_, err := DecodeYAML([]byte(test.document))
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("DecodeYAML() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestDocumentSizeLimit(t *testing.T) {
	document := bytes.Repeat([]byte{' '}, maxDocumentBytes+1)
	if _, err := DecodeJSON(document); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("DecodeJSON() oversized error = %v", err)
	}
	if _, err := DecodeYAML(document); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("DecodeYAML() oversized error = %v", err)
	}
}

func TestExampleProfilesAreValidAndNormalizedDeterministically(t *testing.T) {
	paths := []string{
		filepath.Join("..", "..", "examples", "profiles", "base.yaml"),
		filepath.Join("..", "..", "examples", "profiles", "frontend-node.yaml"),
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			value, err := DecodeYAML(data)
			if err != nil {
				t.Fatalf("DecodeYAML(example) error = %v", err)
			}
			first, err := MarshalYAML(value)
			if err != nil {
				t.Fatalf("MarshalYAML(example) error = %v", err)
			}
			second, err := MarshalYAML(value)
			if err != nil || !bytes.Equal(first, second) {
				t.Fatalf("normalized example is unstable: %v", err)
			}
		})
	}
}
