// Package assessment turns source-backed inventory facts into deterministic,
// read-only recommendations. It never executes commands or writes state.
package assessment

import (
	"github.com/gitbagHero/EnvMason/internal/inventory"
	"github.com/gitbagHero/EnvMason/internal/projectscan"
	"github.com/gitbagHero/EnvMason/internal/versiondata"
)

const PolicySchemaVersion = "0.1.0"

type Channel string

const (
	ChannelLTS    Channel = "lts"
	ChannelStable Channel = "stable"
)

type ToolPolicy struct {
	Channel       Channel `json:"channel,omitempty"`
	Pin           string  `json:"pin,omitempty"`
	IgnoreUpdates bool    `json:"ignore_updates,omitempty"`
}

type Policy struct {
	SchemaVersion string                `json:"schema_version"`
	Tools         map[string]ToolPolicy `json:"tools"`
}

func DefaultPolicy() Policy {
	return Policy{
		SchemaVersion: PolicySchemaVersion,
		Tools: map[string]ToolPolicy{
			"runtime.node": {Channel: ChannelLTS},
			"runtime.java": {Channel: ChannelLTS},
		},
	}
}

type Input struct {
	Inventory   inventory.Inventory
	Versions    versiondata.Result
	Projects    projectscan.Result
	Policy      Policy
	JavaVendors map[string]string
}
