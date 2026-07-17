// Package version provides conservative, deterministic version parsing and comparison.
package version

type Scheme string

const (
	SchemeSemVer Scheme = "semver"
	SchemeJava   Scheme = "java"
)

type Relation string

const (
	RelationLess    Relation = "less"
	RelationEqual   Relation = "equal"
	RelationGreater Relation = "greater"
	RelationUnknown Relation = "unknown"
)

// Value preserves the original input and a canonical representation. An
// incomparable value is explicit and never participates in ordering.
type Value struct {
	Raw        string
	Normalized string
	Scheme     Scheme
	Comparable bool
	semver     *semVersion
	java       *javaVersion
}

// Compare returns Unknown when either value is invalid or the schemes differ.
func Compare(left, right Value) Relation {
	if !left.Comparable || !right.Comparable || left.Scheme != right.Scheme {
		return RelationUnknown
	}
	switch left.Scheme {
	case SchemeSemVer:
		if left.semver == nil || right.semver == nil {
			return RelationUnknown
		}
		return compareSemVer(*left.semver, *right.semver)
	case SchemeJava:
		if left.java == nil || right.java == nil {
			return RelationUnknown
		}
		return compareJava(*left.java, *right.java)
	default:
		return RelationUnknown
	}
}

func invert(relation Relation) Relation {
	switch relation {
	case RelationLess:
		return RelationGreater
	case RelationGreater:
		return RelationLess
	default:
		return relation
	}
}
