package recoverymanifest

import (
	"bytes"
	"reflect"
	"testing"
)

func TestJSONRoundTripAndStrictNonExecutableBoundary(t *testing.T) {
	value := mustRecoveryManifest(t)
	data, err := Marshal(value)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	decoded, err := Decode(data)
	if err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, value) {
		t.Fatal("Recovery Manifest JSON round trip changed the value")
	}
	for _, forbidden := range [][]byte{
		[]byte(`"command"`),
		[]byte(`"args"`),
		[]byte(`"confirmation"`),
		[]byte(`"snapshot"`),
		[]byte(`"facts"`),
		[]byte(`"diff"`),
		[]byte(`"invocation"`),
		[]byte(`"stdout"`),
		[]byte(`"stderr"`),
	} {
		if bytes.Contains(data, forbidden) {
			t.Fatalf("Recovery Manifest contains forbidden field %s", forbidden)
		}
	}

	tests := []struct {
		name string
		data []byte
	}{
		{
			"unknown root field",
			insertBeforeRecoveryEnd(data, `,"command":"restore"`),
		},
		{
			"item confirmation",
			bytes.Replace(
				data,
				[]byte(`"action_id":`),
				[]byte(`"confirmation":{},"action_id":`),
				1,
			),
		},
		{
			"item snapshot",
			bytes.Replace(
				data,
				[]byte(`"action_id":`),
				[]byte(`"snapshot":{"facts":{}},"action_id":`),
				1,
			),
		},
		{
			"wrong schema",
			bytes.Replace(
				data,
				[]byte(`"schema_version": "0.1.0"`),
				[]byte(`"schema_version": "0.2.0"`),
				1,
			),
		},
		{
			"wrong disposition",
			bytes.Replace(
				data,
				[]byte(`"disposition": "manual_action"`),
				[]byte(`"disposition": "prepare_new_plan"`),
				1,
			),
		},
		{
			"evidence state mismatch",
			bytes.Replace(
				data,
				[]byte(`"evidence": "changed"`),
				[]byte(`"evidence": "uncertain"`),
				1,
			),
		},
		{
			"trailing value",
			append(append([]byte{}, data...), []byte("{}")...),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateJSON(test.data); err == nil {
				t.Fatal("ValidateJSON unexpectedly succeeded")
			}
			if _, err := Decode(test.data); err == nil {
				t.Fatal("Decode unexpectedly succeeded")
			}
		})
	}
}

func insertBeforeRecoveryEnd(data []byte, addition string) []byte {
	result := bytes.TrimSpace(append([]byte{}, data...))
	return append(
		result[:len(result)-1],
		append([]byte(addition), '}')...,
	)
}
