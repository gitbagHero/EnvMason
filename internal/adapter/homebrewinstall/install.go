// Package homebrewinstall contains the fixed I21-D Homebrew write adapter.
// It is intentionally separate from the read-only homebrew discovery package.
package homebrewinstall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/gitbagHero/EnvMason/internal/baseinstall"
	"github.com/gitbagHero/EnvMason/internal/execution"
	"github.com/gitbagHero/EnvMason/internal/plan"
)

const (
	InstallTimeout      = 15 * time.Minute
	InstallProbeTimeout = 30 * time.Second
)

var installVersionPattern = regexp.MustCompile(`^[0-9][0-9A-Za-z.+_-]{0,127}$`)

var controlledInstallValues = map[string]string{
	"HOMEBREW_NO_ANALYTICS":                  "1",
	"HOMEBREW_NO_ASK":                        "1",
	"HOMEBREW_NO_AUTO_UPDATE":                "1",
	"HOMEBREW_NO_ENV_HINTS":                  "1",
	"HOMEBREW_NO_INSTALL_CLEANUP":            "1",
	"HOMEBREW_NO_INSTALL_UPGRADE":            "1",
	"HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK": "1",
}

// InstallOptions bind the fixed write definitions to one immutable Plan and
// reviewed Homebrew transaction. Verifier is used only for fixed read-only
// installed-formula probes before and after the registered write process.
type InstallOptions struct {
	CandidatePlan  plan.Plan
	Plan           plan.Plan
	Review         baseinstall.TransactionReview
	BrewPath       string
	ExecutableData []byte
	Configuration  baseinstall.ConfigurationSnapshot
	Verifier       execution.ProcessRunner
}

type installBinding struct {
	action  plan.Action
	preview baseinstall.ActionPreview
}

type formulaStatus uint8

const (
	formulaAbsent formulaStatus = iota
	formulaExact
	formulaConflict
)

type installedFormulae struct {
	versions   map[string][]string
	conflicted map[string]bool
}

// InstallDefinitions exposes only the Git and CMake formula actions already
// sealed by the supplied I21-C review. It never registers arbitrary formulae.
func InstallDefinitions(options InstallOptions) ([]execution.Definition, error) {
	bound, err := cloneInstallOptions(options)
	if err != nil {
		return nil, err
	}
	bindings, err := installBindings(bound)
	if err != nil {
		return nil, err
	}
	result := make([]execution.Definition, 0, len(bindings))
	for _, formula := range []string{"git", "cmake"} {
		binding, ok := bindings[formula]
		if !ok {
			continue
		}
		result = append(result, installDefinition(bound, binding))
	}
	return result, nil
}

func installDefinition(options InstallOptions, binding installBinding) execution.Definition {
	formula := binding.preview.Root.Name
	return execution.Definition{
		Key: execution.ActionKey{
			ToolID: "homebrew.formula." + formula, Operation: "install", Adapter: "homebrew",
		},
		MinimumRisk: plan.RiskR2,
		Build: func(action plan.Action) (execution.CommandSpec, error) {
			if !reflect.DeepEqual(action, binding.action) {
				return execution.CommandSpec{}, errors.New("Homebrew action does not match its reviewed Plan action")
			}
			environment, sensitive := controlledInstallEnvironment(options)
			return execution.CommandSpec{
				Executable:  options.BrewPath,
				Args:        []string{"install", "--formula", "--force-bottle", formula},
				Environment: environment, Directory: installTemporary(options),
				Timeout: InstallTimeout, SensitiveValues: sensitive, TerminateTree: true,
			}, nil
		},
		Preflight: func(ctx context.Context, _ plan.Action) error {
			installed, err := inspectInstalledFormulae(ctx, options)
			if err != nil {
				return err
			}
			return validatePreflight(binding.preview, installed)
		},
		Capture: func(ctx context.Context, _ plan.Action) (execution.Snapshot, error) {
			installed, err := inspectInstalledFormulae(ctx, options)
			if err != nil {
				return execution.Snapshot{}, err
			}
			return installSnapshot(options, binding.preview, installed)
		},
		Satisfied: func(ctx context.Context, _ plan.Action) (bool, error) {
			installed, err := inspectInstalledFormulae(ctx, options)
			if err != nil {
				return false, err
			}
			return validateInstalledClosure(binding.preview, installed) == nil, nil
		},
		Verify: func(ctx context.Context, _ plan.Action, result execution.ProcessResult) error {
			if result.Failure != nil || result.ExitCode == nil || *result.ExitCode != 0 {
				return errors.New("Homebrew install process did not complete successfully")
			}
			installed, err := inspectInstalledFormulae(ctx, options)
			if err != nil {
				return err
			}
			return validateInstalledClosure(binding.preview, installed)
		},
		RevalidateCheckpoint: func(ctx context.Context, action plan.Action, recorded execution.Snapshot) (execution.Snapshot, error) {
			if !reflect.DeepEqual(action, binding.action) ||
				recorded.Facts["plan_id"] != options.Plan.ID ||
				recorded.Facts["review_id"] != options.Review.ID ||
				recorded.Facts["action_id"] != action.ID {
				return execution.Snapshot{}, errors.New("Homebrew checkpoint binding is invalid")
			}
			installed, err := inspectInstalledFormulae(ctx, options)
			if err != nil {
				return execution.Snapshot{}, err
			}
			if err := validateInstalledClosure(binding.preview, installed); err != nil {
				return execution.Snapshot{}, err
			}
			observed, err := installSnapshot(options, binding.preview, installed)
			if err != nil || observed.Digest != recorded.Digest {
				return execution.Snapshot{}, errors.New("Homebrew install checkpoint drifted")
			}
			return observed, nil
		},
	}
}

func cloneInstallOptions(options InstallOptions) (InstallOptions, error) {
	candidateData, err := plan.Marshal(options.CandidatePlan)
	if err != nil {
		return InstallOptions{}, errors.New("Homebrew install options contain an invalid candidate Plan")
	}
	clonedCandidate, err := plan.Decode(candidateData)
	if err != nil {
		return InstallOptions{}, errors.New("Homebrew install options could not copy the candidate Plan")
	}
	data, err := plan.Marshal(options.Plan)
	if err != nil {
		return InstallOptions{}, errors.New("Homebrew install options contain an invalid Plan")
	}
	clonedPlan, err := plan.Decode(data)
	if err != nil {
		return InstallOptions{}, errors.New("Homebrew install options could not copy the Plan")
	}
	reviewData, err := json.Marshal(options.Review)
	if err != nil {
		return InstallOptions{}, errors.New("Homebrew install options could not copy the review")
	}
	var clonedReview baseinstall.TransactionReview
	if err := json.Unmarshal(reviewData, &clonedReview); err != nil ||
		baseinstall.ValidateTransactionReview(clonedReview) != nil {
		return InstallOptions{}, errors.New("Homebrew install options contain an invalid review")
	}
	options.CandidatePlan = clonedCandidate
	options.Plan = clonedPlan
	options.Review = clonedReview
	options.ExecutableData = append([]byte{}, options.ExecutableData...)
	options.Configuration = cloneConfiguration(options.Configuration)
	return options, nil
}

func installBindings(options InstallOptions) (map[string]installBinding, error) {
	if options.Verifier == nil ||
		!safeInstallPath(options.BrewPath) || options.BrewPath != activeHomebrewPath(options.Plan) ||
		!safeInstallPath(installHome(options)) || !safeInstallPath(installTemporary(options)) ||
		baseinstall.ValidateExecutionSnapshots(options.Review, options.ExecutableData, options.Configuration) != nil {
		return nil, errors.New("Homebrew install options are incomplete or not bound to the reviewed Plan")
	}
	if options.Review.PlanID != options.CandidatePlan.ID {
		return nil, errors.New("Homebrew install review is not bound to the candidate Plan")
	}
	want, err := plan.BindBaseTransactionReview(options.CandidatePlan, options.Review.ID)
	if err != nil || !reflect.DeepEqual(want, options.Plan) {
		return nil, errors.New("Homebrew install execution Plan does not match the reviewed candidate")
	}
	actions := make(map[string]plan.Action, len(options.Plan.Actions))
	for _, action := range options.Plan.Actions {
		actions[action.ID] = action
	}
	bindings := make(map[string]installBinding, len(options.Review.Actions))
	for _, preview := range options.Review.Actions {
		action, ok := actions[preview.ActionID]
		formula := preview.Root.Name
		if !ok || (formula != "git" && formula != "cmake") ||
			action.ID != "install-base-"+formula || action.ToolID != "homebrew.formula."+formula ||
			action.Operation != "install" || action.Adapter != "homebrew" ||
			action.TargetVersion != preview.Root.Version || action.Risk != plan.RiskR2 ||
			!action.Confirmation.Required || action.Confirmation.Scope != "plan" ||
			action.ElevationRequired || action.RestartRequired || action.Recovery.Mode != "manual" {
			return nil, errors.New("Homebrew install review does not match the fixed Base action contract")
		}
		if !hasReviewBinding(action, options.Review.ID) {
			return nil, errors.New("Homebrew install Plan is not bound to the supplied review")
		}
		bindings[formula] = installBinding{action: action, preview: preview}
	}
	if len(bindings) != len(options.Plan.Actions) || len(bindings) == 0 {
		return nil, errors.New("Homebrew install review does not exactly cover the Plan")
	}
	return bindings, nil
}

func hasReviewBinding(action plan.Action, reviewID string) bool {
	matches := 0
	for _, check := range action.Preconditions {
		if check.Kind == plan.BaseReviewCheckKind && check.Subject == action.ID && check.Expected == reviewID {
			matches++
		}
	}
	return matches == 1
}

func activeHomebrewPath(value plan.Plan) string {
	for _, installation := range value.Environment.Installations {
		if installation.ID == value.Environment.ActiveInstallationID &&
			installation.Manager == "homebrew" && installation.ActiveState == "active" {
			return installation.Path
		}
	}
	return ""
}

func controlledInstallEnvironment(options InstallOptions) ([]string, []string) {
	temporary := installTemporary(options)
	values := map[string]string{
		"HOME":   installHome(options),
		"PATH":   path.Dir(options.BrewPath) + ":/usr/bin:/bin:/usr/sbin:/sbin",
		"TMPDIR": temporary,
	}
	for key, value := range controlledInstallValues {
		values[key] = value
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	environment := make([]string, 0, len(keys))
	for _, key := range keys {
		environment = append(environment, key+"="+values[key])
	}
	sensitive := []string{installHome(options)}
	if temporary != "/tmp" {
		sensitive = append(sensitive, temporary)
	}
	return environment, sensitive
}

func installHome(options InstallOptions) string {
	return options.Configuration.Environment["HOME"]
}

func installTemporary(options InstallOptions) string {
	return options.Configuration.Environment["TMPDIR"]
}

func safeInstallPath(value string) bool {
	return path.IsAbs(value) && path.Clean(value) == value && !strings.ContainsAny(value, ":\x00\r\n")
}

func cloneConfiguration(value baseinstall.ConfigurationSnapshot) baseinstall.ConfigurationSnapshot {
	result := baseinstall.ConfigurationSnapshot{
		Environment: make(map[string]string, len(value.Environment)),
		System:      append([]byte{}, value.System...), Prefix: append([]byte{}, value.Prefix...), User: append([]byte{}, value.User...),
	}
	for key, entry := range value.Environment {
		result.Environment[key] = entry
	}
	return result
}

func inspectInstalledFormulae(ctx context.Context, options InstallOptions) (installedFormulae, error) {
	versionsOutput, err := runInstalledProbe(ctx, options, []string{"list", "--formula", "--versions"})
	if err != nil {
		return installedFormulae{}, err
	}
	fullNamesOutput, err := runInstalledProbe(ctx, options, []string{"list", "--formula", "--full-name"})
	if err != nil {
		return installedFormulae{}, err
	}
	installed := installedFormulae{
		versions: make(map[string][]string), conflicted: make(map[string]bool),
	}
	seenVersions := make(map[string]bool)
	for _, line := range probeLines(versionsOutput) {
		fields := strings.Fields(line)
		if len(line) > 4096 || len(fields) < 2 || strings.ContainsAny(line, "\x00\r\t") {
			return installedFormulae{}, errors.New("read installed Homebrew formulae: invalid versions output")
		}
		name := fields[0]
		formula, relevant := reviewedFormulaName(options.Review, name)
		if !relevant {
			continue
		}
		if name != formula {
			installed.conflicted[formula] = true
			continue
		}
		if seenVersions[formula] {
			return installedFormulae{}, fmt.Errorf("read installed Homebrew formulae: duplicate formula %q", formula)
		}
		seenVersions[formula] = true
		for _, version := range fields[1:] {
			if !installVersionPattern.MatchString(version) {
				return installedFormulae{}, fmt.Errorf("read installed Homebrew formulae: formula %q has an invalid version", formula)
			}
			installed.versions[formula] = append(installed.versions[formula], version)
		}
	}
	fullNames := make(map[string]bool)
	for _, line := range probeLines(fullNamesOutput) {
		if line == "" || len(line) > 512 || strings.ContainsAny(line, " \t\x00\r") {
			return installedFormulae{}, errors.New("read installed Homebrew formulae: invalid full-name output")
		}
		formula, relevant := reviewedFormulaName(options.Review, line)
		if !relevant {
			continue
		}
		if line != formula {
			installed.conflicted[formula] = true
			continue
		}
		if fullNames[formula] {
			return installedFormulae{}, fmt.Errorf("read installed Homebrew formulae: duplicate full name %q", formula)
		}
		fullNames[formula] = true
	}
	for _, action := range options.Review.Actions {
		formulae := append([]baseinstall.FormulaPreview{action.Root}, action.Dependencies...)
		for _, formula := range formulae {
			_, hasVersions := installed.versions[formula.Name]
			if hasVersions != fullNames[formula.Name] {
				installed.conflicted[formula.Name] = true
			}
		}
	}
	for name, versions := range installed.versions {
		sort.Strings(versions)
		installed.versions[name] = versions
	}
	return installed, nil
}

func runInstalledProbe(ctx context.Context, options InstallOptions, args []string) (string, error) {
	environment, sensitive := controlledInstallEnvironment(options)
	result := options.Verifier.Run(ctx, execution.CommandSpec{
		Executable:  options.BrewPath,
		Args:        append([]string{}, args...),
		Environment: environment, Directory: installTemporary(options),
		Timeout: InstallProbeTimeout, SensitiveValues: sensitive, TerminateTree: true,
	})
	if result.Failure != nil || result.ExitCode == nil || *result.ExitCode != 0 ||
		result.Stdout.Truncated || result.Stderr.Truncated {
		return "", errors.New("read installed Homebrew formulae: fixed probe failed")
	}
	return result.Stdout.Text, nil
}

func probeLines(value string) []string {
	if value == "" {
		return nil
	}
	value = strings.TrimSuffix(value, "\n")
	if value == "" {
		return nil
	}
	return strings.Split(value, "\n")
}

func reviewedFormulaName(review baseinstall.TransactionReview, observed string) (string, bool) {
	base := observed
	if separator := strings.LastIndexByte(observed, '/'); separator >= 0 {
		base = observed[separator+1:]
	}
	for _, action := range review.Actions {
		if action.Root.Name == base {
			return base, true
		}
		for _, dependency := range action.Dependencies {
			if dependency.Name == base {
				return base, true
			}
		}
	}
	return "", false
}

func (installed installedFormulae) status(name, version string) formulaStatus {
	if installed.conflicted[name] {
		return formulaConflict
	}
	versions := installed.versions[name]
	if len(versions) == 0 {
		return formulaAbsent
	}
	if len(versions) == 1 && versions[0] == version {
		return formulaExact
	}
	return formulaConflict
}

func validatePreflight(preview baseinstall.ActionPreview, installed installedFormulae) error {
	if installed.status(preview.Root.Name, preview.Root.Version) == formulaConflict {
		return fmt.Errorf("Homebrew root formula %q has a conflicting installed version", preview.Root.Name)
	}
	for _, dependency := range preview.Dependencies {
		status := installed.status(dependency.Name, dependency.Version)
		if status == formulaConflict ||
			(dependency.State == baseinstall.FormulaSatisfied && status != formulaExact) {
			return fmt.Errorf("Homebrew dependency formula %q drifted after review", dependency.Name)
		}
	}
	return nil
}

func validateInstalledClosure(preview baseinstall.ActionPreview, installed installedFormulae) error {
	formulae := append([]baseinstall.FormulaPreview{preview.Root}, preview.Dependencies...)
	for _, formula := range formulae {
		if installed.status(formula.Name, formula.Version) != formulaExact {
			return fmt.Errorf("Homebrew formula %q did not match the reviewed exact version", formula.Name)
		}
	}
	return nil
}

func installSnapshot(
	options InstallOptions,
	preview baseinstall.ActionPreview,
	installed installedFormulae,
) (execution.Snapshot, error) {
	facts := map[string]string{
		"action_id": preview.ActionID,
		"lock_id":   options.Review.LockID,
		"plan_id":   options.Plan.ID,
		"review_id": options.Review.ID,
	}
	formulae := append([]baseinstall.FormulaPreview{preview.Root}, preview.Dependencies...)
	for _, formula := range formulae {
		value := "absent"
		if installed.conflicted[formula.Name] {
			value = "conflict"
		} else if versions := installed.versions[formula.Name]; len(versions) != 0 {
			value = strings.Join(versions, ",")
		}
		facts["formula."+formula.Name] = value
	}
	return execution.NewSnapshot(facts)
}
