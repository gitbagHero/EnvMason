package execution

import (
	"regexp"
	"sort"
	"strings"
)

const redactedValue = "[REDACTED]"

var secretAssignmentPattern = regexp.MustCompile(`(?i)\b(token|password|secret|authorization)(\s*[:=]\s*)([^\s,;]+)`)

type Redactor struct {
	values []string
}

func NewRedactor(values ...string) Redactor {
	seen := make(map[string]bool, len(values))
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		filtered = append(filtered, value)
	}
	sort.Slice(filtered, func(i, j int) bool { return len(filtered[i]) > len(filtered[j]) })
	return Redactor{values: filtered}
}

func (r Redactor) String(value string) string {
	for _, secret := range r.values {
		value = strings.ReplaceAll(value, secret, redactedValue)
	}
	return secretAssignmentPattern.ReplaceAllString(value, `$1$2`+redactedValue)
}

func (r Redactor) Strings(values []string) []string {
	result := make([]string, len(values))
	for index, value := range values {
		result[index] = r.String(value)
	}
	return result
}
