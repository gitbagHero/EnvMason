package planschema

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestCurrentSchemaIdentityAndCopy(t *testing.T) {
	t.Parallel()
	var document struct {
		ID string `json:"$id"`
	}
	if err := json.Unmarshal(Current(), &document); err != nil || document.ID != ID || Version != "0.3.0" {
		t.Fatalf("schema identity = %#v, %v", document, err)
	}
	first := Current()
	original := bytes.Clone(first)
	first[0] ^= 0xff
	if !bytes.Equal(Current(), original) {
		t.Fatal("mutating returned schema changed embedded data")
	}
	if _, _, ok := ByVersion("9.9.9"); ok {
		t.Fatal("unknown plan schema is available")
	}
	if previous, id, ok := ByVersion(PreviousVersion); !ok || id != PreviousID || len(previous) == 0 {
		t.Fatal("previous plan schema is unavailable")
	}
	if legacy, id, ok := ByVersion(LegacyVersion); !ok || id != LegacyID || len(legacy) == 0 {
		t.Fatal("legacy plan schema is unavailable")
	}
}
