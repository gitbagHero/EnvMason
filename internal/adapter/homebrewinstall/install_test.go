package homebrewinstall

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/gitbagHero/EnvMason/internal/baseinstall"
	"github.com/gitbagHero/EnvMason/internal/execution"
)

func TestInstalledStatusRequiresExactlyOneReviewedVersion(t *testing.T) {
	tests := []struct {
		name     string
		versions []string
		conflict bool
		want     formulaStatus
	}{
		{"absent", nil, false, formulaAbsent},
		{"exact", []string{"2.51.0"}, false, formulaExact},
		{"old", []string{"2.50.0"}, false, formulaConflict},
		{"multiple", []string{"2.50.0", "2.51.0"}, false, formulaConflict},
		{"duplicate", []string{"2.51.0", "2.51.0"}, false, formulaConflict},
		{"metadata conflict", []string{"2.51.0"}, true, formulaConflict},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			installed := installedFormulae{
				versions: map[string][]string{"git": test.versions}, conflicted: map[string]bool{"git": test.conflict},
			}
			if got := installed.status("git", "2.51.0"); got != test.want {
				t.Fatalf("status = %v, want %v", got, test.want)
			}
		})
	}
}

func TestPreflightAndVerificationEnforceReviewedClosure(t *testing.T) {
	preview := baseinstall.ActionPreview{
		ActionID:     "install-base-git",
		Root:         baseinstall.FormulaPreview{Name: "git", Version: "2.51.0", State: baseinstall.FormulaInstallRequired, DownloadBytes: 10},
		Dependencies: []baseinstall.FormulaPreview{{Name: "gettext", Version: "0.26", State: baseinstall.FormulaSatisfied}},
	}
	absentRoot := installedFormulae{
		versions: map[string][]string{"gettext": {"0.26"}}, conflicted: map[string]bool{},
	}
	if err := validatePreflight(preview, absentRoot); err != nil {
		t.Fatalf("valid preflight error = %v", err)
	}
	if err := validateInstalledClosure(preview, absentRoot); err == nil {
		t.Fatal("verification accepted an absent root")
	}
	exact := installedFormulae{
		versions: map[string][]string{"git": {"2.51.0"}, "gettext": {"0.26"}}, conflicted: map[string]bool{},
	}
	if err := validateInstalledClosure(preview, exact); err != nil {
		t.Fatalf("exact closure error = %v", err)
	}
	exact.versions["gettext"] = []string{"0.25"}
	if err := validatePreflight(preview, exact); err == nil || err.Error() == "" {
		t.Fatal("preflight accepted a drifted satisfied dependency")
	}
}

func TestInspectInstalledFormulaeRejectsMalformedDuplicateAndConflictingRecords(t *testing.T) {
	tests := []struct {
		name      string
		versions  string
		fullNames string
	}{
		{"malformed", "git\n", "git\n"},
		{"blank line", "git 2.51.0\n\n", "git\n"},
		{"duplicate", "git 2.51.0\ngit 2.51.0\n", "git\n"},
		{"invalid version", "git latest\n", "git\n"},
		{"NUL", "git 2.51.0\x00\n", "git\n"},
		{"overlong versions line", "git " + strings.Repeat("1", 4093) + "\n", "git\n"},
		{"invalid full name", "git 2.51.0\n", "git bad\n"},
		{"overlong full name", "git 2.51.0\n", strings.Repeat("a", 513) + "/git\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := probeOptions(probeResultRunner{versions: test.versions, fullNames: test.fullNames})
			if _, err := inspectInstalledFormulae(t.Context(), options); err == nil {
				t.Fatal("unsafe installed-formula response was accepted")
			}
		})
	}

	options := probeOptions(probeResultRunner{versions: "git 2.51.0\n", fullNames: "other/tap/git\n"})
	installed, err := inspectInstalledFormulae(t.Context(), options)
	if err != nil || installed.status("git", "2.51.0") != formulaConflict {
		t.Fatalf("foreign tap status/error = %v / %v", installed.status("git", "2.51.0"), err)
	}
}

func TestInspectInstalledFormulaeAcceptsCompactExactAndAbsentOutput(t *testing.T) {
	exact, err := inspectInstalledFormulae(t.Context(), probeOptions(probeResultRunner{
		versions: "git 2.51.0\n", fullNames: "git\n",
	}))
	if err != nil || exact.status("git", "2.51.0") != formulaExact {
		t.Fatalf("exact status/error = %v / %v", exact.status("git", "2.51.0"), err)
	}
	absent, err := inspectInstalledFormulae(t.Context(), probeOptions(probeResultRunner{}))
	if err != nil || absent.status("git", "2.51.0") != formulaAbsent {
		t.Fatalf("absent status/error = %v / %v", absent.status("git", "2.51.0"), err)
	}
	for _, runner := range []probeResultRunner{{failure: true}, {truncated: true}} {
		if _, err := inspectInstalledFormulae(t.Context(), probeOptions(runner)); err == nil {
			t.Fatal("failed or truncated fixed probe was accepted")
		}
	}
}

func probeOptions(runner execution.ProcessRunner) InstallOptions {
	return InstallOptions{
		BrewPath:      "/opt/homebrew/bin/brew",
		Configuration: baseinstall.ConfigurationSnapshot{Environment: map[string]string{"HOME": "/Users/test", "TMPDIR": "/tmp/test"}},
		Review:        baseinstall.TransactionReview{Actions: []baseinstall.ActionPreview{{Root: baseinstall.FormulaPreview{Name: "git"}}}},
		Verifier:      runner,
	}
}

type probeResultRunner struct {
	versions  string
	fullNames string
	failure   bool
	truncated bool
}

func (runner probeResultRunner) Run(_ context.Context, spec execution.CommandSpec) execution.ProcessResult {
	code := 0
	stdout := runner.versions
	if reflect.DeepEqual(spec.Args, []string{"list", "--formula", "--full-name"}) {
		stdout = runner.fullNames
	} else if !reflect.DeepEqual(spec.Args, []string{"list", "--formula", "--versions"}) {
		code = 1
	}
	result := execution.ProcessResult{ExitCode: &code, Stdout: execution.CapturedOutput{Text: stdout, Truncated: runner.truncated}}
	if runner.failure {
		result.Failure = &execution.ExecutionError{Code: execution.CodeExitNonZero, Message: "fixed probe failure"}
	}
	return result
}
