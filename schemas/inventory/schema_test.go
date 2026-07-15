package inventoryschema

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCurrentSchemaIdentity(t *testing.T) {
	t.Parallel()

	var document struct {
		Schema string `json:"$schema"`
		ID     string `json:"$id"`
		Title  string `json:"title"`
	}
	if err := json.Unmarshal(Current(), &document); err != nil {
		t.Fatalf("unmarshal embedded schema: %v", err)
	}
	if document.Schema != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %q", document.Schema)
	}
	if document.ID != ID {
		t.Fatalf("$id = %q, want %q", document.ID, ID)
	}
	if Version != "0.1.0" {
		t.Fatalf("Version = %q", Version)
	}
}

func TestCurrentReturnsCopy(t *testing.T) {
	t.Parallel()

	first := Current()
	original := bytes.Clone(first)
	first[0] ^= 0xff
	if !bytes.Equal(Current(), original) {
		t.Fatal("mutating returned schema changed the embedded schema")
	}
}
