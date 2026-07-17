package version

import (
	"regexp"
	"strings"
)

const maxVersionLength = 256

var semVerPattern = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?$`)

type semVersion struct {
	core       [3]string
	prerelease []string
}

// ParseSemVer parses SemVer 2.0.0 with the common optional Node.js "v" prefix.
func ParseSemVer(raw string) Value {
	value := Value{Raw: raw, Scheme: SchemeSemVer}
	if raw == "" || len(raw) > maxVersionLength || strings.TrimSpace(raw) != raw {
		return value
	}
	matches := semVerPattern.FindStringSubmatch(raw)
	if matches == nil {
		return value
	}
	prerelease := splitIdentifiers(matches[4])
	for _, identifier := range prerelease {
		if numeric(identifier) && len(identifier) > 1 && identifier[0] == '0' {
			return value
		}
	}
	parsed := &semVersion{core: [3]string{matches[1], matches[2], matches[3]}, prerelease: prerelease}
	value.Comparable = true
	value.Normalized = strings.TrimPrefix(raw, "v")
	value.semver = parsed
	return value
}

func compareSemVer(left, right semVersion) Relation {
	for index := range left.core {
		if relation := compareNumeric(left.core[index], right.core[index]); relation != RelationEqual {
			return relation
		}
	}
	if len(left.prerelease) == 0 && len(right.prerelease) == 0 {
		return RelationEqual
	}
	if len(left.prerelease) == 0 {
		return RelationGreater
	}
	if len(right.prerelease) == 0 {
		return RelationLess
	}
	limit := min(len(left.prerelease), len(right.prerelease))
	for index := 0; index < limit; index++ {
		leftID, rightID := left.prerelease[index], right.prerelease[index]
		leftNumeric, rightNumeric := numeric(leftID), numeric(rightID)
		switch {
		case leftNumeric && rightNumeric:
			if relation := compareNumeric(leftID, rightID); relation != RelationEqual {
				return relation
			}
		case leftNumeric:
			return RelationLess
		case rightNumeric:
			return RelationGreater
		case leftID < rightID:
			return RelationLess
		case leftID > rightID:
			return RelationGreater
		}
	}
	switch {
	case len(left.prerelease) < len(right.prerelease):
		return RelationLess
	case len(left.prerelease) > len(right.prerelease):
		return RelationGreater
	default:
		return RelationEqual
	}
}

func splitIdentifiers(value string) []string {
	if value == "" {
		return nil
	}
	return strings.Split(value, ".")
}

func numeric(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func compareNumeric(left, right string) Relation {
	left = trimNumeric(left)
	right = trimNumeric(right)
	switch {
	case len(left) < len(right):
		return RelationLess
	case len(left) > len(right):
		return RelationGreater
	case left < right:
		return RelationLess
	case left > right:
		return RelationGreater
	default:
		return RelationEqual
	}
}

func trimNumeric(value string) string {
	value = strings.TrimLeft(value, "0")
	if value == "" {
		return "0"
	}
	return value
}
