package profile

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	profileschema "github.com/gitbagHero/EnvMason/schemas/profile"
)

const maxDocumentBytes = 1 << 20

var compiledSchema = struct {
	sync.Mutex
	value *jsonschema.Schema
}{}

func DecodeJSON(data []byte) (Profile, error) {
	if err := validateDocumentSize(data); err != nil {
		return Profile{}, err
	}
	if err := checkJSONVersion(data); err != nil {
		return Profile{}, err
	}
	var value Profile
	if err := decodeJSONStrict(data, &value); err != nil {
		return Profile{}, fmt.Errorf("decode profile JSON: %w", err)
	}
	return normalizeAndValidate(value)
}

func DecodeYAML(data []byte) (Profile, error) {
	if err := validateDocumentSize(data); err != nil {
		return Profile{}, err
	}
	if err := inspectYAML(data); err != nil {
		return Profile{}, err
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var value Profile
	if err := decoder.Decode(&value); err != nil {
		return Profile{}, fmt.Errorf("decode profile YAML: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Profile{}, errors.New("decode profile YAML: multiple documents are not allowed")
		}
		return Profile{}, fmt.Errorf("decode profile YAML: %w", err)
	}
	return normalizeAndValidate(value)
}

func MarshalJSON(value Profile) ([]byte, error) {
	normalized, err := normalizeAndValidate(value)
	if err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode profile JSON: %w", err)
	}
	return append(data, '\n'), nil
}

func MarshalYAML(value Profile) ([]byte, error) {
	normalized, err := normalizeAndValidate(value)
	if err != nil {
		return nil, err
	}
	data, err := yaml.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("encode profile YAML: %w", err)
	}
	return data, nil
}

func ValidateJSON(data []byte) error {
	_, err := DecodeJSON(data)
	return err
}

func ValidateYAML(data []byte) error {
	_, err := DecodeYAML(data)
	return err
}

func normalizeAndValidate(value Profile) (Profile, error) {
	normalized, err := Normalize(value)
	if err != nil {
		return Profile{}, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return Profile{}, fmt.Errorf("validate profile: encode normalized value: %w", err)
	}
	if err := validateSchemaJSON(data); err != nil {
		return Profile{}, err
	}
	return normalized, nil
}

func checkJSONVersion(data []byte) error {
	var envelope struct {
		SchemaVersion string `json:"schema_version"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("parse profile JSON: %w", err)
	}
	return validateSchemaVersion(envelope.SchemaVersion)
}

func validateSchemaVersion(version string) error {
	if version == "" {
		return fmt.Errorf(
			"validate profile: schema_version is required; supported version is %q",
			SchemaVersion,
		)
	}
	if version != SchemaVersion {
		return fmt.Errorf(
			"validate profile: unsupported schema_version %q; migration is unavailable; supported version is %q",
			version, SchemaVersion,
		)
	}
	return nil
}

func validateSchemaJSON(data []byte) error {
	schema, err := profileSchema()
	if err != nil {
		return fmt.Errorf("compile profile schema: %w", err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("parse profile JSON: %w", err)
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("validate profile JSON: %w", err)
	}
	return nil
}

func profileSchema() (*jsonschema.Schema, error) {
	compiledSchema.Lock()
	defer compiledSchema.Unlock()
	if compiledSchema.value != nil {
		return compiledSchema.value, nil
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(profileschema.Current()))
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(profileschema.ID, document); err != nil {
		return nil, err
	}
	compiledSchema.value, err = compiler.Compile(profileschema.ID)
	return compiledSchema.value, err
}

func decodeJSONStrict(data []byte, destination any) error {
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

func validateDocumentSize(data []byte) error {
	if len(data) == 0 {
		return errors.New("decode profile: document is empty")
	}
	if len(data) > maxDocumentBytes {
		return fmt.Errorf("decode profile: document exceeds %d bytes", maxDocumentBytes)
	}
	return nil
}

func inspectYAML(data []byte) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("parse profile YAML: %w", err)
	}
	if len(document.Content) != 1 {
		return errors.New("parse profile YAML: exactly one document is required")
	}
	if err := inspectYAMLNode(document.Content[0]); err != nil {
		return err
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("parse profile YAML: multiple documents are not allowed")
		}
		return fmt.Errorf("parse profile YAML: %w", err)
	}
	return nil
}

func inspectYAMLNode(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return errors.New("parse profile YAML: aliases and anchors are not allowed")
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return errors.New("parse profile YAML: explicit tags are not allowed")
	}
	if node.Kind == yaml.MappingNode {
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return errors.New("parse profile YAML: mapping keys must be strings")
			}
			if _, exists := seen[key.Value]; exists {
				return fmt.Errorf("parse profile YAML: duplicate key %q", key.Value)
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := inspectYAMLNode(child); err != nil {
			return err
		}
	}
	return nil
}
