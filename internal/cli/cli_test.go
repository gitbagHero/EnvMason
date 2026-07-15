package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/buildinfo"
)

var testInfo = buildinfo.Info{
	Version:   "1.2.3-test",
	Commit:    "abc123",
	BuildTime: "2026-07-15T12:00:00Z",
	GoVersion: "go1.25.0",
	Target:    "testos/testarch",
}

func TestHelpEntryPoints(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{name: "no arguments"},
		{name: "help command", args: []string{"help"}},
		{name: "short help flag", args: []string{"-h"}},
		{name: "long help flag", args: []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := executeForTest(tt.args)
			if code != ExitSuccess {
				t.Fatalf("exit code = %d, want %d", code, ExitSuccess)
			}
			if stderr != "" {
				t.Fatalf("stderr = %q, want empty", stderr)
			}
			for _, want := range []string{"Usage:", "envmason [command]", "--version", "version"} {
				if !strings.Contains(stdout, want) {
					t.Errorf("stdout does not contain %q:\n%s", want, stdout)
				}
			}
			if strings.Contains(stdout, "completion") {
				t.Errorf("help exposes out-of-scope completion command:\n%s", stdout)
			}
			if strings.Contains(stdout, "-v, --version") {
				t.Errorf("help exposes reserved -v shorthand:\n%s", stdout)
			}
		})
	}
}

func TestVersionEntryPoints(t *testing.T) {
	want := "envmason 1.2.3-test\n" +
		"commit: abc123\n" +
		"built: 2026-07-15T12:00:00Z\n" +
		"go: go1.25.0\n" +
		"target: testos/testarch\n"

	for _, args := range [][]string{{"version"}, {"--version"}} {
		code, stdout, stderr := executeForTest(args)
		if code != ExitSuccess {
			t.Fatalf("Execute(%q) exit code = %d, want %d", args, code, ExitSuccess)
		}
		if stdout != want {
			t.Fatalf("Execute(%q) stdout = %q, want %q", args, stdout, want)
		}
		if stderr != "" {
			t.Fatalf("Execute(%q) stderr = %q, want empty", args, stderr)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "unknown command", args: []string{"scan"}, wantErr: `unknown command "scan" for "envmason"`},
		{name: "version extra argument", args: []string{"version", "extra"}, wantErr: `unknown command "extra"`},
		{name: "unknown flag", args: []string{"--json"}, wantErr: "unknown flag: --json"},
		{name: "reserved short version flag", args: []string{"-v"}, wantErr: "unknown shorthand flag: 'v'"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := executeForTest(tt.args)
			if code != ExitUsage {
				t.Fatalf("exit code = %d, want %d", code, ExitUsage)
			}
			if stdout != "" {
				t.Fatalf("stdout = %q, want empty", stdout)
			}
			if !strings.Contains(stderr, tt.wantErr) {
				t.Errorf("stderr does not contain %q: %s", tt.wantErr, stderr)
			}
			if !strings.HasSuffix(stderr, "Run 'envmason help' for usage.\n") {
				t.Errorf("stderr does not end with usage hint: %s", stderr)
			}
		})
	}
}

func executeForTest(args []string) (int, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := Execute(args, &stdout, &stderr, testInfo)
	return code, stdout.String(), stderr.String()
}
