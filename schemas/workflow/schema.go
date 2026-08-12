// Package workflowschema exposes the versioned EnvMason staged-workflow
// manifest and record schemas.
package workflowschema

import (
	"bytes"
	_ "embed"
)

const (
	ManifestVersion = "0.1.0"
	ManifestID      = "urn:envmason:schema:workflow-manifest:0.1.0"
	RecordVersion   = "0.1.0"
	RecordID        = "urn:envmason:schema:workflow-record:0.1.0"
)

//go:embed manifest-v0.1.0.json
var manifest []byte

//go:embed record-v0.1.0.json
var record []byte

func Manifest() []byte { return bytes.Clone(manifest) }

func Record() []byte { return bytes.Clone(record) }
