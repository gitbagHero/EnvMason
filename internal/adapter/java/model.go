// Package java provides read-only discovery for JDKs, jenv, Maven, and Gradle.
package java

import (
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

type State string

const (
	StateInstalled    State = "installed"
	StateNotInstalled State = "not_installed"
	StateUnknown      State = "unknown"
)

type Manager string

const (
	ManagerSystem   Manager = "system"
	ManagerHomebrew Manager = "homebrew"
	ManagerUnknown  Manager = "unknown"
)

type Request struct {
	PathDirectories     []string
	WorkingDirectory    string
	Home                string
	JavaHome            string
	JavaHomeTool        string
	JenvRoot            string
	JenvShellVersion    string
	HomebrewPrefixes    []string
	GradleUserHome      string
	CollectedAt         time.Time
	ProcessArchitecture inventory.Architecture
}

type JDKInstallation struct {
	ID           string
	Version      string
	Home         string
	Name         string
	Vendor       string
	Architecture inventory.Architecture
	Manager      Manager
	Registered   bool
	JenvAliases  []string
}

type Runtime struct {
	State        State
	Path         string
	ResolvedPath string
	Version      string
	Home         string
	Vendor       string
	Architecture inventory.Architecture
	JDKID        string
}

type JenvRegistration struct {
	Alias  string
	Home   string
	Broken bool
}

type Jenv struct {
	State            State
	Root             string
	Loaded           bool
	GlobalVersion    string
	LocalVersion     string
	LocalVersionFile string
	ShellVersion     string
	EffectiveVersion string
	Registrations    []JenvRegistration
}

type BuildTool struct {
	State        State
	Name         string
	Version      string
	Path         string
	ResolvedPath string
	Home         string
	JavaVersion  string
	JavaHome     string
}

type Result struct {
	State    State
	JavaHome string
	JDKs     []JDKInstallation
	Current  Runtime
	Jenv     Jenv
	Maven    BuildTool
	Gradle   BuildTool
	Findings []inventory.Finding
}
