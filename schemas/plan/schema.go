// Package planschema exposes the versioned EnvMason Plan schema.
package planschema

import (
	"bytes"
	_ "embed"
)

const (
	Version         = "0.3.0"
	ID              = "urn:envmason:schema:plan:0.3.0"
	PreviousVersion = "0.2.0"
	PreviousID      = "urn:envmason:schema:plan:0.2.0"
	LegacyVersion   = "0.1.0"
	LegacyID        = "urn:envmason:schema:plan:0.1.0"
)

//go:embed v0.3.0.json
var current []byte

//go:embed v0.2.0.json
var previous []byte

//go:embed v0.1.0.json
var legacy []byte

func Current() []byte { return bytes.Clone(current) }

func ByVersion(version string) ([]byte, string, bool) {
	switch version {
	case Version:
		return bytes.Clone(current), ID, true
	case PreviousVersion:
		return bytes.Clone(previous), PreviousID, true
	case LegacyVersion:
		return bytes.Clone(legacy), LegacyID, true
	default:
		return nil, "", false
	}
}
