package plan

import (
	"bytes"
	"fmt"
	"strings"
)

type Format string

const (
	FormatSummary Format = "summary"
	FormatJSON    Format = "json"
)

func Render(value Plan, format Format) ([]byte, error) {
	if err := Validate(value); err != nil {
		return nil, err
	}
	switch format {
	case "", FormatSummary:
		return renderSummary(value), nil
	case FormatJSON:
		return Marshal(value)
	default:
		return nil, fmt.Errorf("unsupported plan format %q", format)
	}
}

func renderSummary(value Plan) []byte {
	var output bytes.Buffer
	fmt.Fprintln(&output, "EnvMason plan preview")
	fmt.Fprintf(&output, "Plan ID: %s\n", plain(value.ID))
	fmt.Fprintf(&output, "Created: %s\n", value.CreatedAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(&output, "Expires: %s\n", value.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"))
	fmt.Fprintf(&output, "Executable: %t\n", value.Executable)
	fmt.Fprintf(&output, "Environment: %s %s, %s=%s via %s\n", plain(value.Environment.OS), plain(value.Environment.Architecture), plain(value.Environment.ToolID), plain(value.Environment.ActiveVersion), plain(value.Environment.ActiveManager))
	fmt.Fprintf(&output, "Environment digest: %s\n", plain(value.EnvironmentDigest))
	fmt.Fprintf(&output, "Policy digest: %s\n", plain(value.PolicyDigest))
	fmt.Fprintf(&output, "Summary: %s\n", plain(value.Summary))
	fmt.Fprintln(&output, "Actions:")
	for _, action := range value.Actions {
		fmt.Fprintf(&output, "  %s: %s %s via %s | risk=%s | confirmation=%s | elevation=%t | restart=%t\n", plain(action.ID), plain(action.Operation), plain(action.TargetVersion), plain(action.Adapter), action.Risk, plain(action.Confirmation.Scope), action.ElevationRequired, action.RestartRequired)
		fmt.Fprintf(&output, "    preconditions: %d | verifications: %d | recovery: %s\n", len(action.Preconditions), len(action.Verifications), plain(action.Recovery.Mode))
		if value.Executable {
			for _, check := range action.Preconditions {
				fmt.Fprintf(&output, "    require %s %s = %s\n", plain(check.Kind), plain(check.Subject), plain(check.Expected))
			}
			for _, check := range action.Verifications {
				fmt.Fprintf(&output, "    verify %s %s = %s\n", plain(check.Kind), plain(check.Subject), plain(check.Expected))
			}
		}
	}
	if value.Executable {
		fmt.Fprintln(&output, "This executable Plan requires exact plan-level confirmation before any action can run.")
	} else {
		fmt.Fprintln(&output, "This I13 preview cannot execute or modify the system.")
	}
	return output.Bytes()
}

func plain(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\n' || character == '\r' || character == '\t' {
			return ' '
		}
		if character < 0x20 || character == 0x7f {
			return -1
		}
		return character
	}, value)
}
