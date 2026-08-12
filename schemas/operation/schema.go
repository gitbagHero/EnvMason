// Package operationschema exposes the versioned EnvMason operation-record schema.
package operationschema

import (
	"bytes"
	_ "embed"
)

const (
	Version         = "0.4.0"
	PreviousVersion = "0.3.0"
	OlderVersion    = "0.2.0"
	LegacyVersion   = "0.1.0"
	ID              = "urn:envmason:schema:operation:0.4.0"
	PreviousID      = "urn:envmason:schema:operation:0.3.0"
	OlderID         = "urn:envmason:schema:operation:0.2.0"
	LegacyID        = "urn:envmason:schema:operation:0.1.0"
)

//go:embed v0.4.0.json
var current []byte

//go:embed v0.3.0.json
var previous []byte

//go:embed v0.2.0.json
var older []byte

//go:embed v0.1.0.json
var legacy []byte

func Current() []byte { return bytes.Clone(current) }

func ByVersion(version string) ([]byte, string, bool) {
	switch version {
	case Version:
		return bytes.Clone(current), ID, true
	case PreviousVersion:
		return bytes.Clone(previous), PreviousID, true
	case OlderVersion:
		return bytes.Clone(older), OlderID, true
	case LegacyVersion:
		return bytes.Clone(legacy), LegacyID, true
	default:
		return nil, "", false
	}
}
