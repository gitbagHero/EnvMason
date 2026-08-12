package recoveryschema

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestManifestSchemaIdentityCopyAndCompilation(t *testing.T) {
	first := Manifest()
	second := Manifest()
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("embedded Recovery Manifest schema is empty or unstable")
	}
	first[0] ^= 0xff
	if bytes.Equal(first, Manifest()) {
		t.Fatal("Manifest returned mutable shared storage")
	}
	var envelope struct {
		ID string `json:"$id"`
	}
	if err := json.Unmarshal(second, &envelope); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	if envelope.ID != ID {
		t.Fatalf("schema ID = %q, want %q", envelope.ID, ID)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(second))
	if err != nil {
		t.Fatalf("parse schema document: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(ID, document); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	if _, err := compiler.Compile(ID); err != nil {
		t.Fatalf("compile schema: %v", err)
	}
}
