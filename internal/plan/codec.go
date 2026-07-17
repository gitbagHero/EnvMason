package plan

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	planschema "github.com/gitbagHero/EnvMason/schemas/plan"
)

var compiledSchemas = struct {
	sync.Mutex
	values map[string]*jsonschema.Schema
}{values: make(map[string]*jsonschema.Schema)}

func Marshal(value Plan) ([]byte, error) {
	if err := Validate(value); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode plan: %w", err)
	}
	data = append(data, '\n')
	if err := ValidateJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func Decode(data []byte) (Plan, error) {
	if err := ValidateJSON(data); err != nil {
		return Plan{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value Plan
	if err := decoder.Decode(&value); err != nil {
		return Plan{}, fmt.Errorf("decode plan: %w", err)
	}
	if _, err := decoder.Token(); err == nil || err != io.EOF {
		return Plan{}, errorsTrailingJSON()
	}
	if err := Validate(value); err != nil {
		return Plan{}, err
	}
	return value, nil
}

func ValidateJSON(data []byte) error {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("parse plan JSON: %w", err)
	}
	if envelope.SchemaVersion != SchemaVersion && envelope.SchemaVersion != ExecutableSchemaVersion {
		return fmt.Errorf("validate plan JSON: unsupported schema_version %q", envelope.SchemaVersion)
	}
	schema, err := schemaForVersion(envelope.SchemaVersion)
	if err != nil {
		return fmt.Errorf("compile plan schema: %w", err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse plan JSON: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate plan JSON: %w", err)
	}
	return nil
}

func schemaForVersion(version string) (*jsonschema.Schema, error) {
	compiledSchemas.Lock()
	defer compiledSchemas.Unlock()
	if schema := compiledSchemas.values[version]; schema != nil {
		return schema, nil
	}
	data, id, ok := planschema.ByVersion(version)
	if !ok {
		return nil, fmt.Errorf("unsupported schema_version %q", version)
	}
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
	schema, err := compiler.Compile(id)
	if err != nil {
		return nil, err
	}
	compiledSchemas.values[version] = schema
	return schema, nil
}

func errorsTrailingJSON() error { return fmt.Errorf("decode plan: trailing JSON value") }
