package lockschema

import (
	"bytes"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestCurrentIsVersionedCompilableAndIndependent(t *testing.T) {
	first := Current()
	second := Current()
	if len(first) == 0 || !bytes.Equal(first, second) {
		t.Fatal("embedded Lock schema is empty or unstable")
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
	compileLockSchema(t, second)
}

func TestSchemaAcceptsLockShapeAndRejectsExecutableFields(t *testing.T) {
	schema := compileLockSchema(t, Current())
	valid := []byte(`{
  "schema_version":"0.1.0",
  "id":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "generated_at":"2026-08-10T07:00:00Z",
  "executable":false,
  "confirmable":false,
  "profile":{"schema_version":"0.1.0","name":"base","digest":"sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
  "target":{"os":"macos","os_version":"15.0","architecture":"arm64"},
  "sources":[{"id":"homebrew-core","kind":"package_catalog","uri":"https://formulae.brew.sh/api/formula.json","snapshot_at":"2026-08-10T06:00:00Z","digest":"sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}],
  "items":[{"id":"base.git","module":"base","capability":"base.git","state":"install_required","implementation":{"tool_id":"base.git","manager":"homebrew","package_kind":"formula","package_id":"git","version":"2.51.0","source_id":"homebrew-core","conditions":{"os":"macos","architectures":["amd64","arm64"]}},"observed":[],"reason":"installation_required"}],
  "summary":{"satisfied":0,"install_required":1,"conflict":0,"unresolved":0}
}`)
	validateSchemaDocument(t, schema, valid, true)

	for name, replacement := range map[string][]byte{
		"command":     bytes.Replace(valid, []byte(`"confirmable":false,`), []byte(`"confirmable":false,"command":"whoami",`), 1),
		"path":        bytes.Replace(valid, []byte(`"observed":[]`), []byte(`"observed":[],"path":"/Users/alice"`), 1),
		"confirmable": bytes.Replace(valid, []byte(`"confirmable":false`), []byte(`"confirmable":true`), 1),
		"bad state":   bytes.Replace(valid, []byte(`"state":"install_required"`), []byte(`"state":"planned"`), 1),
	} {
		t.Run(name, func(t *testing.T) { validateSchemaDocument(t, schema, replacement, false) })
	}
}

func compileLockSchema(t *testing.T, data []byte) *jsonschema.Schema {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse Lock schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	if err := compiler.AddResource(ID, document); err != nil {
		t.Fatalf("add Lock schema: %v", err)
	}
	schema, err := compiler.Compile(ID)
	if err != nil {
		t.Fatalf("compile Lock schema: %v", err)
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
		t.Fatalf("valid Lock rejected: %v", err)
	}
	if !valid && err == nil {
		t.Fatal("invalid Lock accepted")
	}
}
