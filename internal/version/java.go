package version

import (
	"regexp"
	"strings"
)

var javaUpdatePattern = regexp.MustCompile(`^([0-9]+)u([0-9]+)(?:-b([0-9]+))?(?:-([0-9A-Za-z.-]+))?$`)

var javaTags = map[string]bool{
	"amzn": true, "corretto": true, "graalvm": true, "homebrew": true,
	"librca": true, "lts": true, "microsoft": true, "ms": true,
	"openjdk": true, "openj9": true, "oracle": true, "redhat": true,
	"tem": true, "temurin": true, "zulu": true,
}

type javaVersion struct {
	components []string
	early      bool
	build      string
}

// ParseJava parses modern Java versions, legacy 1.x versions, common 8u
// versions, early-access builds, and a bounded set of vendor/support tags.
func ParseJava(raw string) Value {
	value := Value{Raw: raw, Scheme: SchemeJava}
	if raw == "" || len(raw) > maxVersionLength || strings.TrimSpace(raw) != raw {
		return value
	}
	if matches := javaUpdatePattern.FindStringSubmatch(raw); matches != nil {
		components := []string{canonicalNumber(matches[1]), "0", canonicalNumber(matches[2])}
		if !validJavaTags(matches[4]) {
			return value
		}
		parsed := &javaVersion{components: components, build: canonicalOptionalNumber(matches[3])}
		value.Comparable, value.java = true, parsed
		value.Normalized = normalizedJava(components, false, parsed.build, matches[4])
		return value
	}

	baseAndTag, buildAndTag, ok := cutOnce(raw, "+")
	if !ok {
		return value
	}
	base, suffix := cutJavaSuffix(baseAndTag)
	build, buildTag, ok := parseJavaBuild(buildAndTag)
	if !ok {
		return value
	}
	components, ok := parseJavaComponents(base)
	if !ok {
		return value
	}

	early := false
	tags := []string{}
	for _, tag := range append(splitTags(suffix), splitTags(buildTag)...) {
		tag = strings.ToLower(tag)
		switch {
		case tag == "ea":
			early = true
		case strings.HasPrefix(tag, "b") && numeric(strings.TrimPrefix(tag, "b")) && build == "":
			build = canonicalNumber(strings.TrimPrefix(tag, "b"))
		case javaTags[tag]:
			tags = append(tags, tag)
		default:
			return value
		}
	}
	parsed := &javaVersion{components: components, early: early, build: build}
	value.Comparable, value.java = true, parsed
	value.Normalized = normalizedJava(components, early, build, strings.Join(tags, "-"))
	return value
}

func compareJava(left, right javaVersion) Relation {
	limit := max(len(left.components), len(right.components))
	for index := 0; index < limit; index++ {
		leftPart, rightPart := "0", "0"
		if index < len(left.components) {
			leftPart = left.components[index]
		}
		if index < len(right.components) {
			rightPart = right.components[index]
		}
		if relation := compareNumeric(leftPart, rightPart); relation != RelationEqual {
			return relation
		}
	}
	if left.early != right.early {
		if left.early {
			return RelationLess
		}
		return RelationGreater
	}
	if left.early && right.early && left.build != "" && right.build != "" {
		return compareNumeric(left.build, right.build)
	}
	return RelationEqual
}

func parseJavaComponents(base string) ([]string, bool) {
	if base == "" || strings.Count(base, "_") > 1 ||
		strings.HasPrefix(base, ".") || strings.HasPrefix(base, "_") ||
		strings.HasSuffix(base, ".") || strings.HasSuffix(base, "_") ||
		strings.Contains(base, "..") || strings.Contains(base, "._") ||
		strings.Contains(base, "_.") || strings.Contains(base, "__") {
		return nil, false
	}
	parts := strings.FieldsFunc(base, func(character rune) bool { return character == '.' || character == '_' })
	if len(parts) == 0 || len(parts) > 6 {
		return nil, false
	}
	for _, part := range parts {
		if !numeric(part) {
			return nil, false
		}
	}
	if strings.HasPrefix(base, "1.") {
		if len(parts) < 2 || parts[0] != "1" {
			return nil, false
		}
		parts = parts[1:]
	}
	components := make([]string, len(parts))
	for index, part := range parts {
		components[index] = canonicalNumber(part)
	}
	return components, true
}

func cutJavaSuffix(value string) (string, string) {
	base, suffix, found := strings.Cut(value, "-")
	if !found {
		return value, ""
	}
	return base, suffix
}

func parseJavaBuild(value string) (string, string, bool) {
	if value == "" {
		return "", "", true
	}
	build, tag, found := strings.Cut(value, "-")
	if !numeric(build) {
		return "", "", false
	}
	if !found {
		tag = ""
	}
	return canonicalNumber(build), tag, true
}

func cutOnce(value, separator string) (string, string, bool) {
	if strings.Count(value, separator) > 1 {
		return "", "", false
	}
	left, right, found := strings.Cut(value, separator)
	if !found {
		return value, "", true
	}
	return left, right, true
}

func splitTags(value string) []string {
	if value == "" {
		return nil
	}
	return strings.FieldsFunc(value, func(character rune) bool { return character == '-' || character == '.' })
}

func validJavaTags(value string) bool {
	for _, tag := range splitTags(value) {
		if !javaTags[strings.ToLower(tag)] {
			return false
		}
	}
	return true
}

func normalizedJava(components []string, early bool, build, tags string) string {
	result := strings.Join(components, ".")
	if early {
		result += "-ea"
	}
	if build != "" {
		result += "+" + build
	}
	if tags != "" {
		result += "-" + strings.ToLower(tags)
	}
	return result
}

func canonicalNumber(value string) string { return trimNumeric(value) }

func canonicalOptionalNumber(value string) string {
	if value == "" {
		return ""
	}
	return canonicalNumber(value)
}
