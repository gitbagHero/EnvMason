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
	if Version != "0.2.0" {
		t.Fatalf("Version = %q", Version)
	}
}

func TestLegacySchemaRemainsAvailable(t *testing.T) {
	t.Parallel()

	data, id, ok := ByVersion(LegacyVersion)
	if !ok {
		t.Fatal("legacy schema is unavailable")
	}
	if id != LegacyID {
		t.Fatalf("legacy ID = %q, want %q", id, LegacyID)
	}
	var document struct {
		ID string `json:"$id"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("unmarshal legacy schema: %v", err)
	}
	if document.ID != LegacyID {
		t.Fatalf("legacy $id = %q, want %q", document.ID, LegacyID)
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

	legacy, _, _ := ByVersion(LegacyVersion)
	legacyOriginal := bytes.Clone(legacy)
	legacy[0] ^= 0xff
	legacyAfterMutation, _, _ := ByVersion(LegacyVersion)
	if !bytes.Equal(legacyAfterMutation, legacyOriginal) {
		t.Fatal("mutating returned legacy schema changed the embedded schema")
	}
}
