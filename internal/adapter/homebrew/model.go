// Package homebrew provides a read-only Homebrew adapter.
package homebrew

import (
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const JSONFormatV2 = "homebrew-json-v2"

type State string

const (
	StateInstalled    State = "installed"
	StateNotInstalled State = "not_installed"
	StateUnknown      State = "unknown"
)

type PackageKind string

const (
	PackageFormula PackageKind = "formula"
	PackageCask    PackageKind = "cask"
)

type Request struct {
	PathDirectories     []string
	WorkingDirectory    string
	Home                string
	CollectedAt         time.Time
	ProcessArchitecture inventory.Architecture
}

type OutdatedPackage struct {
	Kind              PackageKind
	Name              string
	InstalledVersions []string
	CurrentVersion    string
	Pinned            bool
	PinnedVersion     string
}

type Result struct {
	State        State
	BrewPath     string
	ResolvedPath string
	Version      string
	Prefix       string
	Repository   string
	Cellar       string
	Caskroom     string
	Origin       string
	Architecture inventory.Architecture
	DataFormat   string
	Tools        []inventory.Tool
	Outdated     []OutdatedPackage
	Findings     []inventory.Finding
}
