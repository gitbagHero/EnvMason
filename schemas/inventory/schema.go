// Package inventoryschema exposes the versioned EnvMason inventory schema.
package inventoryschema

import (
	"bytes"
	_ "embed"
)

const (
	Version       = "0.2.0"
	ID            = "urn:envmason:schema:inventory:0.2.0"
	LegacyVersion = "0.1.0"
	LegacyID      = "urn:envmason:schema:inventory:0.1.0"
)

//go:embed v0.2.0.json
var current []byte

//go:embed v0.1.0.json
var legacy []byte

// Current returns a copy of the current schema document.
func Current() []byte {
	return bytes.Clone(current)
}

// ByVersion returns a copy of a supported schema and its canonical ID.
func ByVersion(version string) ([]byte, string, bool) {
	switch version {
	case Version:
		return bytes.Clone(current), ID, true
	case LegacyVersion:
		return bytes.Clone(legacy), LegacyID, true
	default:
		return nil, "", false
	}
}
