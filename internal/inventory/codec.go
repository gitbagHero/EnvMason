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

var compiledSchema = sync.OnceValues(compileSchema)

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
	schema, err := compiledSchema()
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

func compileSchema() (*jsonschema.Schema, error) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(inventoryschema.Current()))
	if err != nil {
		return nil, fmt.Errorf("parse embedded schema: %w", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(inventoryschema.ID, document); err != nil {
		return nil, fmt.Errorf("add embedded schema: %w", err)
	}
	return compiler.Compile(inventoryschema.ID)
}
