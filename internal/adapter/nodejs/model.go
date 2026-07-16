// Package nodejs provides read-only discovery for Node.js and its package managers.
package nodejs

import (
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

type Manager string

const (
	ManagerSystem   Manager = "system"
	ManagerHomebrew Manager = "homebrew"
	ManagerNVM      Manager = "nvm"
	ManagerUnknown  Manager = "unknown"
)

type State string

const (
	StateInstalled    State = "installed"
	StateNotInstalled State = "not_installed"
	StateUnknown      State = "unknown"
)

type Request struct {
	PathDirectories     []string
	WorkingDirectory    string
	Home                string
	XDGConfigHome       string
	NVMDirectory        string
	NVMBin              string
	HomebrewPrefixes    []string
	CollectedAt         time.Time
	ProcessArchitecture inventory.Architecture
}

type NodeInstallation struct {
	ID                string
	Version           string
	NormalizedVersion string
	Path              string
	ResolvedPath      string
	Architecture      inventory.Architecture
	Manager           Manager
	Effective         bool
	Default           bool
	InPATH            bool
}

type PackageManager struct {
	Name               string
	Version            string
	Path               string
	ResolvedPath       string
	Manager            Manager
	NodeInstallationID string
	Effective          bool
	CorepackProxy      bool
	ProviderVersion    string
}

type NVM struct {
	State          State
	Directory      string
	Loaded         bool
	DefaultAlias   string
	DefaultVersion string
}

type Result struct {
	State           State
	NVM             NVM
	CurrentNodeID   string
	Nodes           []NodeInstallation
	PackageManagers []PackageManager
	Findings        []inventory.Finding
}
