package workflow

import (
	"bytes"
	"reflect"
	"testing"
)

func TestManifestJSONRoundTripAndStrictBoundary(t *testing.T) {
	manifest := mustManifest(t)
	data, err := MarshalManifest(manifest)
	if err != nil {
		t.Fatalf("MarshalManifest() error = %v", err)
	}
	decoded, err := DecodeManifest(data)
	if err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, manifest) {
		t.Fatal("manifest JSON round trip changed the value")
	}
	for _, forbidden := range [][]byte{
		[]byte(`"command"`),
		[]byte(`"args"`),
		[]byte(`"confirmation"`),
		[]byte(`"runner"`),
	} {
		if bytes.Contains(data, forbidden) {
			t.Fatalf("manifest contains forbidden field %s", forbidden)
		}
	}

	tests := []struct {
		name string
		data []byte
	}{
		{
			"unknown root field",
			insertBeforeLastObjectEnd(data, `,"confirmation":{}`),
		},
		{
			"stage command",
			bytes.Replace(
				data,
				[]byte(`"id": "install_node"`),
				[]byte(`"id": "install_node", "command": "nvm"`),
				1,
			),
		},
		{
			"wrong version",
			bytes.Replace(data, []byte(`"schema_version": "0.1.0"`), []byte(`"schema_version": "0.2.0"`), 1),
		},
		{
			"invalid risk",
			bytes.Replace(data, []byte(`"risk": "R2"`), []byte(`"risk": "R3"`), 1),
		},
		{
			"trailing value",
			append(append([]byte{}, data...), []byte("{}")...),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateManifestJSON(test.data); err == nil {
				t.Fatal("ValidateManifestJSON unexpectedly succeeded")
			}
			if _, err := DecodeManifest(test.data); err == nil {
				t.Fatal("DecodeManifest unexpectedly succeeded")
			}
		})
	}
}

func TestRecordJSONRoundTripAndStrictBoundary(t *testing.T) {
	manifest := mustManifest(t)
	record := runningAtStage(t, manifest, 2)
	completed, err := FinishStage(
		record,
		manifest,
		StageUpdateNodeTools,
		StageCompleted,
		digestID('f'),
		record.UpdatedAt.Add(1),
	)
	if err != nil {
		t.Fatalf("FinishStage() error = %v", err)
	}
	data, err := MarshalRecord(completed, manifest)
	if err != nil {
		t.Fatalf("MarshalRecord() error = %v", err)
	}
	decoded, err := DecodeRecord(data, manifest)
	if err != nil {
		t.Fatalf("DecodeRecord() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, completed) {
		t.Fatal("record JSON round trip changed the value")
	}
	for _, forbidden := range [][]byte{
		[]byte(`"command"`),
		[]byte(`"args"`),
		[]byte(`"confirmation"`),
		[]byte(`"runner"`),
	} {
		if bytes.Contains(data, forbidden) {
			t.Fatalf("record contains forbidden field %s", forbidden)
		}
	}

	tests := []struct {
		name string
		data []byte
	}{
		{
			"unknown root field",
			insertBeforeLastObjectEnd(data, `,"confirmation":{}`),
		},
		{
			"stage args",
			bytes.Replace(
				data,
				[]byte(`"id": "install_node"`),
				[]byte(`"id": "install_node", "args": []`),
				1,
			),
		},
		{
			"unknown state",
			bytes.Replace(data, []byte(`"state": "completed"`), []byte(`"state": "unknown"`), 1),
		},
		{
			"wrong stage Plan schema",
			bytes.Replace(
				data,
				[]byte(`"plan_schema_version": "0.2.0"`),
				[]byte(`"plan_schema_version": "0.3.0"`),
				1,
			),
		},
		{
			"wrong version",
			bytes.Replace(data, []byte(`"schema_version": "0.1.0"`), []byte(`"schema_version": "0.2.0"`), 1),
		},
		{
			"trailing value",
			append(append([]byte{}, data...), []byte("{}")...),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateRecordJSON(test.data); err == nil {
				t.Fatal("ValidateRecordJSON unexpectedly succeeded")
			}
			if _, err := DecodeRecord(test.data, manifest); err == nil {
				t.Fatal("DecodeRecord unexpectedly succeeded")
			}
		})
	}
}

func TestDecodeRecordRequiresMatchingManifest(t *testing.T) {
	manifest := mustManifest(t)
	record := mustNewRecord(t, manifest)
	data, err := MarshalRecord(record, manifest)
	if err != nil {
		t.Fatalf("MarshalRecord() error = %v", err)
	}
	input := testBuildInput()
	input.TargetNodeVersion = "24.15.0"
	other, err := BuildManifest(input)
	if err != nil {
		t.Fatalf("BuildManifest(other) error = %v", err)
	}
	if _, err := DecodeRecord(data, other); err == nil {
		t.Fatal("record decoded against a different manifest")
	}
}

func insertBeforeLastObjectEnd(data []byte, addition string) []byte {
	result := bytes.TrimSpace(append([]byte{}, data...))
	result = append(result[:len(result)-1], append([]byte(addition), '}')...)
	return result
}
