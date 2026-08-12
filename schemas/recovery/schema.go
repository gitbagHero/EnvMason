// Package recoveryschema exposes the versioned EnvMason Recovery Manifest
// schema.
package recoveryschema

import (
	"bytes"
	_ "embed"
)

const (
	Version = "0.1.0"
	ID      = "urn:envmason:schema:recovery-manifest:0.1.0"
)

//go:embed manifest-v0.1.0.json
var manifest []byte

func Manifest() []byte { return bytes.Clone(manifest) }
