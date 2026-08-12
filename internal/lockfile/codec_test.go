package lockfile

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestMarshalDecodeRoundTrip(t *testing.T) {
	value, err := Build(validBuildInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	data, err := Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !bytes.HasSuffix(data, []byte("\n")) || bytes.Contains(data, []byte("/Users/")) {
		t.Fatalf("Lock JSON is not normalized or contains a private path: %s", data)
	}
	decoded, err := Decode(data)
	if err != nil || !reflect.DeepEqual(decoded, value) {
		t.Fatalf("Decode() = %#v, %v", decoded, err)
	}
}

func TestDecodeRejectsVersionInjectionAndTrailingValues(t *testing.T) {
	value, err := Build(validBuildInput())
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	valid, err := Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	tests := []struct {
		name     string
		document []byte
		message  string
	}{
		{"missing version", bytes.Replace(valid, []byte(`"schema_version": "0.1.0",`), nil, 1), "unsupported schema_version"},
		{"future version", bytes.Replace(valid, []byte(`"schema_version": "0.1.0"`), []byte(`"schema_version": "0.2.0"`), 1), "migration is unavailable"},
		{"command", bytes.Replace(valid, []byte(`"confirmable": false,`), []byte(`"confirmable": false, "command": "whoami",`), 1), "additional properties"},
		{"path", bytes.Replace(valid, []byte(`"reason": "installation_required"`), []byte(`"reason": "installation_required", "path": "/Users/alice"`), 1), "additional properties"},
		{"trailing", append(append([]byte{}, valid...), []byte(` {}`)...), "invalid character"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Decode(test.document)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.message)) {
				t.Fatalf("Decode() error = %v, want containing %q", err, test.message)
			}
		})
	}
}

func TestDecodeRejectsOversizedDocument(t *testing.T) {
	_, err := Decode(bytes.Repeat([]byte{' '}, (1<<20)+1))
	if err == nil || !strings.Contains(err.Error(), "1-1048576") {
		t.Fatalf("Decode() error = %v", err)
	}
}
