package recoverymanifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	recoveryschema "github.com/gitbagHero/EnvMason/schemas/recovery"
)

var compiledSchema = struct {
	sync.Mutex
	value *jsonschema.Schema
}{}

func Marshal(value Manifest) ([]byte, error) {
	if err := Validate(value); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Recovery Manifest: %w", err)
	}
	data = append(data, '\n')
	if err := ValidateJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func Decode(data []byte) (Manifest, error) {
	if err := ValidateJSON(data); err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value Manifest
	if err := decoder.Decode(&value); err != nil {
		return Manifest{}, fmt.Errorf("decode Recovery Manifest: %w", err)
	}
	if _, err := decoder.Token(); err == nil {
		return Manifest{}, errors.New(
			"decode Recovery Manifest: trailing JSON value",
		)
	} else if !errors.Is(err, io.EOF) {
		return Manifest{}, fmt.Errorf("decode Recovery Manifest: %w", err)
	}
	if err := Validate(value); err != nil {
		return Manifest{}, err
	}
	return value, nil
}

func ValidateJSON(data []byte) error {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("parse Recovery Manifest JSON: %w", err)
	}
	if envelope.SchemaVersion != SchemaVersion {
		return fmt.Errorf(
			"validate Recovery Manifest JSON: unsupported schema_version %q",
			envelope.SchemaVersion,
		)
	}
	schema, err := schema()
	if err != nil {
		return fmt.Errorf("compile Recovery Manifest schema: %w", err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse Recovery Manifest JSON: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate Recovery Manifest JSON: %w", err)
	}
	return nil
}

func schema() (*jsonschema.Schema, error) {
	compiledSchema.Lock()
	defer compiledSchema.Unlock()
	if compiledSchema.value != nil {
		return compiledSchema.value, nil
	}
	document, err := jsonschema.UnmarshalJSON(
		bytes.NewReader(recoveryschema.Manifest()),
	)
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(recoveryschema.ID, document); err != nil {
		return nil, err
	}
	value, err := compiler.Compile(recoveryschema.ID)
	if err != nil {
		return nil, err
	}
	compiledSchema.value = value
	return value, nil
}
