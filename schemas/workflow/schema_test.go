package workflowschema

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestSchemasExposeVersionedIndependentCopies(t *testing.T) {
	tests := []struct {
		name    string
		version string
		id      string
		load    func() []byte
	}{
		{
			name: "manifest", version: ManifestVersion,
			id: ManifestID, load: Manifest,
		},
		{
			name: "record", version: RecordVersion,
			id: RecordID, load: Record,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := test.load()
			second := test.load()
			if len(first) == 0 || !bytes.Equal(first, second) {
				t.Fatal("embedded schema is empty or unstable")
			}
			first[0] ^= 0xff
			if bytes.Equal(first, test.load()) {
				t.Fatal("schema loader returned mutable shared storage")
			}
			var envelope struct {
				ID string `json:"$id"`
			}
			if err := json.Unmarshal(second, &envelope); err != nil {
				t.Fatalf("parse schema: %v", err)
			}
			if envelope.ID != test.id {
				t.Fatalf("schema ID = %q, want %q", envelope.ID, test.id)
			}
			compiler := jsonschema.NewCompiler()
			compiler.DefaultDraft(jsonschema.Draft2020)
			compiler.AssertFormat()
			document, err := jsonschema.UnmarshalJSON(bytes.NewReader(second))
			if err != nil {
				t.Fatalf("parse schema document: %v", err)
			}
			if err := compiler.AddResource(test.id, document); err != nil {
				t.Fatalf("add schema: %v", err)
			}
			if _, err := compiler.Compile(test.id); err != nil {
				t.Fatalf("compile schema: %v", err)
			}
		})
	}
}
