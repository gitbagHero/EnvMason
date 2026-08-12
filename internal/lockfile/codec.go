package lockfile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"

	lockschema "github.com/gitbagHero/EnvMason/schemas/lock"
)

var compiledSchema = struct {
	sync.Mutex
	value *jsonschema.Schema
}{}

func Marshal(value Lock) ([]byte, error) {
	if err := Validate(value); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode Lock: %w", err)
	}
	data = append(data, '\n')
	if err := validateSchemaJSON(data); err != nil {
		return nil, err
	}
	return data, nil
}

func Decode(data []byte) (Lock, error) {
	if len(data) == 0 || len(data) > 1<<20 {
		return Lock{}, errors.New("decode Lock: document must contain 1-1048576 bytes")
	}
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Lock{}, fmt.Errorf("parse Lock JSON: %w", err)
	}
	if envelope.SchemaVersion != SchemaVersion {
		return Lock{}, fmt.Errorf(
			"validate Lock JSON: unsupported schema_version %q; migration is unavailable; supported version is %q",
			envelope.SchemaVersion, SchemaVersion,
		)
	}
	if err := validateSchemaJSON(data); err != nil {
		return Lock{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var value Lock
	if err := decoder.Decode(&value); err != nil {
		return Lock{}, fmt.Errorf("decode Lock JSON: %w", err)
	}
	if _, err := decoder.Token(); err == nil {
		return Lock{}, errors.New("decode Lock JSON: trailing JSON value")
	} else if !errors.Is(err, io.EOF) {
		return Lock{}, fmt.Errorf("decode Lock JSON: %w", err)
	}
	if err := Validate(value); err != nil {
		return Lock{}, err
	}
	return value, nil
}

func ValidateJSON(data []byte) error {
	_, err := Decode(data)
	return err
}

func validateSchemaJSON(data []byte) error {
	schema, err := lockSchema()
	if err != nil {
		return fmt.Errorf("compile Lock schema: %w", err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse Lock JSON: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate Lock JSON: %w", err)
	}
	return nil
}

func lockSchema() (*jsonschema.Schema, error) {
	compiledSchema.Lock()
	defer compiledSchema.Unlock()
	if compiledSchema.value != nil {
		return compiledSchema.value, nil
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(lockschema.Current()))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(lockschema.ID, document); err != nil {
		return nil, err
	}
	compiledSchema.value, err = compiler.Compile(lockschema.ID)
	return compiledSchema.value, err
}
