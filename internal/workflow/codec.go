package workflow

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	workflowschema "github.com/gitbagHero/EnvMason/schemas/workflow"
)

var compiledSchemas = struct {
	sync.Mutex
	manifest *jsonschema.Schema
	record   *jsonschema.Schema
}{}

func MarshalManifest(value Manifest) ([]byte, error) {
	if err := ValidateManifest(value); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode workflow manifest: %w", err)
	}
	data = append(data, '\n')
	if err := ValidateManifestJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func DecodeManifest(data []byte) (Manifest, error) {
	if err := ValidateManifestJSON(data); err != nil {
		return Manifest{}, err
	}
	var value Manifest
	if err := decodeStrict(data, &value); err != nil {
		return Manifest{}, fmt.Errorf("decode workflow manifest: %w", err)
	}
	if err := ValidateManifest(value); err != nil {
		return Manifest{}, err
	}
	return value, nil
}

func ValidateManifestJSON(data []byte) error {
	return validateJSON(
		data,
		ManifestSchemaVersion,
		"workflow manifest",
		manifestSchema,
	)
}

func MarshalRecord(value Record, manifest Manifest) ([]byte, error) {
	if err := ValidateRecord(value, manifest); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode workflow record: %w", err)
	}
	data = append(data, '\n')
	if err := ValidateRecordJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func DecodeRecord(data []byte, manifest Manifest) (Record, error) {
	if err := ValidateRecordJSON(data); err != nil {
		return Record{}, err
	}
	var value Record
	if err := decodeStrict(data, &value); err != nil {
		return Record{}, fmt.Errorf("decode workflow record: %w", err)
	}
	if err := ValidateRecord(value, manifest); err != nil {
		return Record{}, err
	}
	return value, nil
}

func ValidateRecordJSON(data []byte) error {
	return validateJSON(
		data,
		RecordSchemaVersion,
		"workflow record",
		recordSchema,
	)
}

func validateJSON(
	data []byte,
	expectedVersion string,
	name string,
	schemaFor func() (*jsonschema.Schema, error),
) error {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("parse %s JSON: %w", name, err)
	}
	if envelope.SchemaVersion != expectedVersion {
		return fmt.Errorf(
			"validate %s JSON: unsupported schema_version %q",
			name,
			envelope.SchemaVersion,
		)
	}
	schema, err := schemaFor()
	if err != nil {
		return fmt.Errorf("compile %s schema: %w", name, err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse %s JSON: %w", name, err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate %s JSON: %w", name, err)
	}
	return nil
}

func manifestSchema() (*jsonschema.Schema, error) {
	compiledSchemas.Lock()
	defer compiledSchemas.Unlock()
	if compiledSchemas.manifest != nil {
		return compiledSchemas.manifest, nil
	}
	schema, err := compileSchema(
		workflowschema.Manifest(),
		workflowschema.ManifestID,
	)
	if err != nil {
		return nil, err
	}
	compiledSchemas.manifest = schema
	return schema, nil
}

func recordSchema() (*jsonschema.Schema, error) {
	compiledSchemas.Lock()
	defer compiledSchemas.Unlock()
	if compiledSchemas.record != nil {
		return compiledSchemas.record, nil
	}
	schema, err := compileSchema(
		workflowschema.Record(),
		workflowschema.RecordID,
	)
	if err != nil {
		return nil, err
	}
	compiledSchemas.record = schema
	return schema, nil
}

func compileSchema(data []byte, id string) (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(id, document); err != nil {
		return nil, err
	}
	return compiler.Compile(id)
}

func decodeStrict(data []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if _, err := decoder.Token(); err == nil {
		return errors.New("trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}
