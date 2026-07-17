package inventory

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	inventoryschema "github.com/gitbagHero/EnvMason/schemas/inventory"
)

var compiledSchemas = struct {
	sync.Mutex
	values map[string]*jsonschema.Schema
}{values: make(map[string]*jsonschema.Schema)}

// Marshal validates inventory against the public schema and emits stable,
// indented JSON with a trailing newline.
func Marshal(value Inventory) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode inventory: %w", err)
	}
	data = append(data, '\n')
	if err := ValidateJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

// Decode validates JSON against the public schema and decodes it into the Go
// model. Unknown fields are rejected by both the schema and Go decoder.
func Decode(data []byte) (Inventory, error) {
	if err := ValidateJSON(data); err != nil {
		return Inventory{}, err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var value Inventory
	if err := decoder.Decode(&value); err != nil {
		return Inventory{}, fmt.Errorf("decode inventory: %w", err)
	}
	if _, err := decoder.Token(); err == nil || err != io.EOF {
		return Inventory{}, fmt.Errorf("decode inventory: trailing JSON value")
	}
	return value, nil
}

// ValidateJSON validates one JSON document against the embedded inventory
// schema. It never resolves schema resources over the network.
func ValidateJSON(data []byte) error {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("parse inventory JSON: %w", err)
	}
	if envelope.SchemaVersion == "" {
		return fmt.Errorf("validate inventory JSON: missing schema_version")
	}

	schema, err := schemaForVersion(envelope.SchemaVersion)
	if err != nil {
		return fmt.Errorf("compile inventory schema: %w", err)
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse inventory JSON: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate inventory JSON: %w", err)
	}
	return nil
}

func schemaForVersion(version string) (*jsonschema.Schema, error) {
	compiledSchemas.Lock()
	defer compiledSchemas.Unlock()
	if schema := compiledSchemas.values[version]; schema != nil {
		return schema, nil
	}

	data, id, ok := inventoryschema.ByVersion(version)
	if !ok {
		return nil, fmt.Errorf("unsupported schema_version %q", version)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse embedded schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if version == SchemaVersion {
		previous, previousID, ok := inventoryschema.ByVersion(PreviousSchemaVersion)
		if !ok {
			return nil, fmt.Errorf("load previous inventory schema %q", PreviousSchemaVersion)
		}
		previousDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(previous))
		if err != nil {
			return nil, fmt.Errorf("parse previous embedded schema: %w", err)
		}
		if err := compiler.AddResource(previousID, previousDocument); err != nil {
			return nil, fmt.Errorf("add previous embedded schema: %w", err)
		}
	}
	if err := compiler.AddResource(id, document); err != nil {
		return nil, fmt.Errorf("add embedded schema: %w", err)
	}
	schema, err := compiler.Compile(id)
	if err != nil {
		return nil, err
	}
	compiledSchemas.values[version] = schema
	return schema, nil
}
