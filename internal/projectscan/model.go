// Package projectscan finds static Node.js and Java version references in
// explicitly selected project trees without executing project code.
package projectscan

import "time"

type Runtime string

const (
	RuntimeNode Runtime = "node"
	RuntimeJava Runtime = "java"
)

type ConstraintKind string

const (
	ConstraintExact   ConstraintKind = "exact"
	ConstraintRange   ConstraintKind = "range"
	ConstraintAlias   ConstraintKind = "alias"
	ConstraintUnknown ConstraintKind = "unknown"
)

type Reference struct {
	Runtime    Runtime
	Constraint string
	Normalized string
	Kind       ConstraintKind
	File       string
}

type Project struct {
	ID         string
	Root       string
	References []Reference
}

type Issue struct {
	Code    string
	Root    string
	File    string
	Runtime Runtime
	Details []string
}

type Result struct {
	CollectedAt time.Time
	Projects    []Project
	Issues      []Issue
}

type Request struct {
	Roots            []string
	Excludes         []string
	WorkingDirectory string
	Home             string
	CollectedAt      time.Time
}
