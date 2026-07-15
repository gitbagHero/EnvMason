// Package inventoryschema exposes the versioned EnvMason inventory schema.
package inventoryschema

import (
	"bytes"
	_ "embed"
)

const (
	Version = "0.1.0"
	ID      = "urn:envmason:schema:inventory:0.1.0"
)

//go:embed v0.1.0.json
var current []byte

// Current returns a copy of the current schema document.
func Current() []byte {
	return bytes.Clone(current)
}
