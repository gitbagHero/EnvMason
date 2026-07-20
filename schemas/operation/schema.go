// Package operationschema exposes the versioned EnvMason operation-record schema.
package operationschema

import (
	"bytes"
	_ "embed"
)

const (
	Version         = "0.2.0"
	PreviousVersion = "0.1.0"
	ID              = "urn:envmason:schema:operation:0.2.0"
	PreviousID      = "urn:envmason:schema:operation:0.1.0"
)

//go:embed v0.2.0.json
var current []byte

//go:embed v0.1.0.json
var previous []byte

func Current() []byte { return bytes.Clone(current) }

func ByVersion(version string) ([]byte, string, bool) {
	switch version {
	case Version:
		return bytes.Clone(current), ID, true
	case PreviousVersion:
		return bytes.Clone(previous), PreviousID, true
	default:
		return nil, "", false
	}
}
