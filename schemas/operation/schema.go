// Package operationschema exposes the versioned EnvMason operation-record schema.
package operationschema

import (
	"bytes"
	_ "embed"
)

const (
	Version = "0.1.0"
	ID      = "urn:envmason:schema:operation:0.1.0"
)

//go:embed v0.1.0.json
var current []byte

func Current() []byte { return bytes.Clone(current) }

func ByVersion(version string) ([]byte, string, bool) {
	if version != Version {
		return nil, "", false
	}
	return bytes.Clone(current), ID, true
}
