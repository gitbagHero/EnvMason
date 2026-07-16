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
	accessPath        string
	invocationPath    string
}

// InvocationPath returns the unredacted path encountered in PATH order. It is
// intended only for bounded invocation by another deterministic core component.
func (c Candidate) InvocationPath() string {
	return c.invocationPath
}

// AccessPath returns the unredacted resolved path for another deterministic
// core component. It must never be copied into reports or logs directly.
func (c Candidate) AccessPath() string {
	return c.accessPath
}

// Result retains candidates in PATH order and records non-fatal findings.
type Result struct {
	Command    string
	Candidates []Candidate
	Findings   []inventory.Finding
}
