// Package executable discovers command candidates without executing them.
package executable

import (
	"time"

	"github.com/gitbagHero/EnvMason/internal/inventory"
)

const UnknownPath = "unknown"

type LinkState string

const (
	LinkStateNotLink  LinkState = "not_link"
	LinkStateResolved LinkState = "resolved"
	LinkStateBroken   LinkState = "broken"
	LinkStateLoop     LinkState = "loop"
	LinkStateUnknown  LinkState = "unknown"
)

// Request contains all environment-dependent inputs required for discovery.
// Directories retain PATH order and may contain duplicates.
type Request struct {
	Command          string
	Directories      []string
	WorkingDirectory string
	Home             string
	CollectedAt      time.Time
}

// Candidate is one command-shaped file encountered while walking PATH order.
type Candidate struct {
	DirectoryPosition int
	Path              string
	ResolvedPath      string
	LinkState         LinkState
	Architectures     []inventory.Architecture
	Executable        bool
	Effective         bool
	Duplicate         bool
}

// Result retains candidates in PATH order and records non-fatal findings.
type Result struct {
	Command    string
	Candidates []Candidate
	Findings   []inventory.Finding
}
