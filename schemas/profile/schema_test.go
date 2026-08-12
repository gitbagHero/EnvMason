package profileschema

import (
	"bytes"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCurrentIsVersionedCompilableAndIndependent(t *testing.T) {
	first := Current()
	second := Current()
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("embedded Profile schema is empty or unstable")
	}
	first[0] ^= 0xff
	if bytes.Equal(first, Current()) {
		t.Fatal("Current returned mutable shared storage")
	}
	if data, id, ok := ByVersion(Version); !ok || id != ID || !bytes.Equal(data, second) {
		t.Fatalf("ByVersion(%q) returned ok=%t id=%q", Version, ok, id)
	}
	if _, _, ok := ByVersion("0.2.0"); ok {
		t.Fatal("ByVersion accepted an unsupported version")
	}
	compileProfileSchema(t, second)
}

func TestSchemaAcceptsNormalizedProfileAndRejectsExecutableOrInvalidShapes(t *testing.T) {
	schema := compileProfileSchema(t, Current())
	valid := []byte(`{
  "schema_version":"0.1.0",
  "name":"frontend-node",
  "modules":[
    {"id":"base","variant":"standard","options":{"build_tools":true,"terminal_configuration":true}},
    {"id":"frontend_node","variant":"minimal","version_policy":{"strategy":"exact","pin":"22.14.0"},"options":{"corepack":false,"pnpm":true,"browser_testing":false}}
  ]
}`)
	validateSchemaDocument(t, schema, valid, true)

	invalid := map[string][]byte{
		"command":        []byte(`{"schema_version":"0.1.0","name":"unsafe","command":"curl example.invalid","modules":[{"id":"base"}]}`),
		"unknown module": []byte(`{"schema_version":"0.1.0","name":"unknown","modules":[{"id":"backend_java"}]}`),
		"base version":   []byte(`{"schema_version":"0.1.0","name":"base-version","modules":[{"id":"base","version_policy":{"strategy":"lts"}}]}`),
		"missing pin":    []byte(`{"schema_version":"0.1.0","name":"missing-pin","modules":[{"id":"frontend_node","version_policy":{"strategy":"exact"}}]}`),
		"floating pin":   []byte(`{"schema_version":"0.1.0","name":"floating-pin","modules":[{"id":"frontend_node","version_policy":{"strategy":"exact","pin":"22"}}]}`),
	}
	for name, document := range invalid {
		t.Run(name, func(t *testing.T) {
			validateSchemaDocument(t, schema, document, false)
		})
	}
}

func compileProfileSchema(t *testing.T, data []byte) *jsonschema.Schema {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse Profile schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(ID, document); err != nil {
		t.Fatalf("add Profile schema: %v", err)
	}
	schema, err := compiler.Compile(ID)
	if err != nil {
		t.Fatalf("compile Profile schema: %v", err)
	}
	return schema
}

func validateSchemaDocument(t *testing.T, schema *jsonschema.Schema, data []byte, valid bool) {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse test document: %v", err)
	}
	err = schema.Validate(document)
	if valid && err != nil {
		t.Fatalf("valid Profile rejected: %v", err)
	}
	if !valid && err == nil {
		t.Fatal("invalid Profile accepted")
	}
}
