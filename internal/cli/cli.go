package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	applypkg "github.com/gitbagHero/EnvMason/internal/apply"
	"github.com/gitbagHero/EnvMason/internal/buildinfo"
	defaultpkg "github.com/gitbagHero/EnvMason/internal/defaultversion"
	"github.com/gitbagHero/EnvMason/internal/execution"
	planpkg "github.com/gitbagHero/EnvMason/internal/plan"
	"github.com/gitbagHero/EnvMason/internal/report"
)

const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Execute runs the EnvMason CLI with explicit inputs and outputs so command
// behavior can be tested without changing process-global state.
func Execute(args []string, stdout, stderr io.Writer, info buildinfo.Info) int {
	service := applypkg.DefaultService()
	defaultService := defaultpkg.DefaultService()
	return execute(args, stdout, stderr, info, commandDependencies{
		generateReport: report.Generate, generatePlan: planpkg.Generate,
		prepareApply: service.Prepare, executeApply: service.Execute, confirmApply: confirmPlan,
		prepareDefaultSet: defaultService.PrepareSet, prepareDefaultRestore: defaultService.PrepareRestore,
		executeDefault: defaultService.Execute, confirmDefault: confirmExplicit,
	})
}

func execute(args []string, stdout, stderr io.Writer, info buildinfo.Info, deps commandDependencies) int {
	root := newRootCommand(info, stdout, stderr, deps)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		var operational operationalError
		if errors.As(err, &operational) {
			return ExitFailure
		}
		fmt.Fprintln(stderr, "Run 'envmason help' for usage.")
		return ExitUsage
	}

	return ExitSuccess
}

type commandDependencies struct {
	generateReport        func(context.Context, report.Options) ([]byte, error)
	generatePlan          func(context.Context, planpkg.Options) ([]byte, error)
	prepareApply          func(context.Context, applypkg.Options) (applypkg.Prepared, error)
	executeApply          func(context.Context, applypkg.Prepared, execution.ConfirmationReceipt) (applypkg.Result, error)
	confirmApply          func(string) (execution.ConfirmationReceipt, error)
	prepareDefaultSet     func(context.Context, defaultpkg.SetOptions) (defaultpkg.Prepared, error)
	prepareDefaultRestore func(context.Context, defaultpkg.RestoreOptions) (defaultpkg.Prepared, error)
	executeDefault        func(context.Context, defaultpkg.Prepared, execution.ConfirmationReceipt) (defaultpkg.Result, error)
	confirmDefault        func(string, string) (execution.ConfirmationReceipt, error)
}

type operationalError struct{ err error }

func (e operationalError) Error() string { return e.err.Error() }
func (e operationalError) Unwrap() error { return e.err }

func newRootCommand(info buildinfo.Info, stdout, stderr io.Writer, deps commandDependencies) *cobra.Command {
	var showVersion bool

	root := &cobra.Command{
		Use:                   "envmason",
		Short:                 "Manage developer workstation lifecycles safely",
		SilenceErrors:         true,
		SilenceUsage:          true,
		DisableSuggestions:    true,
		DisableFlagsInUseLine: true,
		Args:                  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if showVersion {
				fmt.Fprint(cmd.OutOrStdout(), formatVersion(info))
				return nil
			}
			return cmd.Help()
		},
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.CompletionOptions.DisableDefaultCmd = true
	root.Flags().BoolVar(&showVersion, "version", false, "print version and build information")

	root.AddCommand(&cobra.Command{
		Use:                   "version",
		Short:                 "Print version and build information",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		Run: func(cmd *cobra.Command, _ []string) {
			fmt.Fprint(cmd.OutOrStdout(), formatVersion(info))
		},
	})
	root.AddCommand(newReportCommand(deps))
	root.AddCommand(newPlanCommand(deps))
	root.AddCommand(newApplyCommand(deps))
	root.AddCommand(newDefaultCommand(deps))

	return root
}

func newApplyCommand(deps commandDependencies) *cobra.Command {
	var toolID string
	var version string
	var online bool
	var dryRun bool
	command := &cobra.Command{
		Use:                   "apply",
		Short:                 "Review and apply one confirmed Node.js NVM installation Plan",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options := applypkg.Options{ToolID: toolID, Version: version, Online: online}
			if err := applypkg.ValidateOptions(options); err != nil {
				return err
			}
			if deps.prepareApply == nil || deps.executeApply == nil || deps.confirmApply == nil {
				return operationalError{err: errors.New("apply dependencies are unavailable")}
			}
			prepared, err := deps.prepareApply(cmd.Context(), options)
			if err != nil {
				return operationalError{err: fmt.Errorf("prepare executable Plan: %w", err)}
			}
			data, err := planpkg.Render(prepared.Plan, planpkg.FormatSummary)
			if err != nil {
				return operationalError{err: fmt.Errorf("render executable Plan: %w", err)}
			}
			if _, err := cmd.OutOrStdout().Write(data); err != nil {
				return operationalError{err: fmt.Errorf("write executable Plan: %w", err)}
			}
			if dryRun {
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run complete; no action or operation record was created.")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Type 'apply %s' to confirm this exact Plan: ", prepared.Plan.ID)
			receipt, err := deps.confirmApply(prepared.Plan.ID)
			if err != nil {
				return operationalError{err: err}
			}
			result, err := deps.executeApply(cmd.Context(), prepared, receipt)
			if result.RecordPath != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Operation record: %s\n", result.RecordPath)
			}
			if err != nil {
				return operationalError{err: fmt.Errorf("execute confirmed Plan: %w", err)}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Operation %s completed and verified.\n", result.Record.ID)
			return nil
		},
	}
	command.Flags().StringVar(&toolID, "tool", "", "tool ID; I15 supports runtime.node")
	command.Flags().StringVar(&version, "version", "", "exact stable Node.js version to install")
	command.Flags().BoolVar(&online, "online", false, "require fresh official Node.js release evidence")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "review the executable Plan without confirmation or writes")
	return command
}

func confirmPlan(planID string) (execution.ConfirmationReceipt, error) {
	return confirmExplicit("apply", planID)
}

func confirmExplicit(phrase, planID string) (execution.ConfirmationReceipt, error) {
	info, err := os.Stdin.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return execution.ConfirmationReceipt{}, errors.New("interactive plan-level confirmation is required")
	}
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 256), 1024)
	if !scanner.Scan() {
		return execution.ConfirmationReceipt{}, errors.New("Plan was not confirmed")
	}
	if scanner.Text() != phrase+" "+planID {
		return execution.ConfirmationReceipt{}, errors.New("Plan was not confirmed; no action was executed")
	}
	return execution.ConfirmationReceipt{Scope: "plan", ConfirmedPlanID: planID, ConfirmedAt: time.Now().UTC()}, nil
}

func newDefaultCommand(deps commandDependencies) *cobra.Command {
	command := &cobra.Command{
		Use:                   "default",
		Short:                 "Review and explicitly confirm an R3 runtime default change",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE:                  func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	command.AddCommand(newDefaultSetCommand(deps), newDefaultRestoreCommand(deps))
	return command
}

func newDefaultSetCommand(deps commandDependencies) *cobra.Command {
	var toolID, version string
	var dryRun bool
	command := &cobra.Command{
		Use:                   "set",
		Short:                 "Set an installed NVM Node.js version as the default",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options := defaultpkg.SetOptions{ToolID: toolID, Version: version}
			if err := defaultpkg.ValidateSetOptions(options); err != nil {
				return err
			}
			if deps.prepareDefaultSet == nil || deps.executeDefault == nil || deps.confirmDefault == nil {
				return operationalError{err: errors.New("default set dependencies are unavailable")}
			}
			prepared, err := deps.prepareDefaultSet(cmd.Context(), options)
			if err != nil {
				return operationalError{err: fmt.Errorf("prepare default set Plan: %w", err)}
			}
			if err := renderPreparedPlan(cmd, prepared.Plan, dryRun); err != nil || dryRun {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Type 'set-default %s' to explicitly confirm this R3 Plan: ", prepared.Plan.ID)
			receipt, err := deps.confirmDefault("set-default", prepared.Plan.ID)
			if err != nil {
				return operationalError{err: err}
			}
			result, err := deps.executeDefault(cmd.Context(), prepared, receipt)
			printDefaultRecord(cmd, result)
			if err != nil {
				if recoverableDefaultRecord(result.Record) {
					fmt.Fprintf(cmd.OutOrStdout(), "Recovery review: envmason default restore --operation %s --dry-run\n", result.Record.ID)
				}
				return operationalError{err: fmt.Errorf("execute confirmed default set Plan: %w", err)}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Operation %s completed and verified.\n", result.Record.ID)
			return nil
		},
	}
	command.Flags().StringVar(&toolID, "tool", "", "tool ID; I16 supports runtime.node")
	command.Flags().StringVar(&version, "version", "", "exact installed Node.js version to set as the NVM default")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "review the R3 Plan without confirmation or writes")
	return command
}

func newDefaultRestoreCommand(deps commandDependencies) *cobra.Command {
	var operationID string
	var dryRun bool
	command := &cobra.Command{
		Use:                   "restore",
		Short:                 "Restore the NVM default saved by an I16 operation",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options := defaultpkg.RestoreOptions{OperationID: operationID}
			if err := defaultpkg.ValidateRestoreOptions(options); err != nil {
				return err
			}
			if deps.prepareDefaultRestore == nil || deps.executeDefault == nil || deps.confirmDefault == nil {
				return operationalError{err: errors.New("default restore dependencies are unavailable")}
			}
			prepared, err := deps.prepareDefaultRestore(cmd.Context(), options)
			if err != nil {
				return operationalError{err: fmt.Errorf("prepare default restore Plan: %w", err)}
			}
			if err := renderPreparedPlan(cmd, prepared.Plan, dryRun); err != nil || dryRun {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Type 'restore-default %s' to explicitly confirm this R3 recovery Plan: ", prepared.Plan.ID)
			receipt, err := deps.confirmDefault("restore-default", prepared.Plan.ID)
			if err != nil {
				return operationalError{err: err}
			}
			result, err := deps.executeDefault(cmd.Context(), prepared, receipt)
			printDefaultRecord(cmd, result)
			if err != nil {
				return operationalError{err: fmt.Errorf("execute confirmed default restore Plan: %w", err)}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Operation %s completed and verified.\n", result.Record.ID)
			return nil
		},
	}
	command.Flags().StringVar(&operationID, "operation", "", "source I16 set-default operation ID")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "review the R3 recovery Plan without confirmation or writes")
	return command
}

func renderPreparedPlan(cmd *cobra.Command, value planpkg.Plan, dryRun bool) error {
	data, err := planpkg.Render(value, planpkg.FormatSummary)
	if err != nil {
		return operationalError{err: fmt.Errorf("render executable Plan: %w", err)}
	}
	if _, err := cmd.OutOrStdout().Write(data); err != nil {
		return operationalError{err: fmt.Errorf("write executable Plan: %w", err)}
	}
	if dryRun {
		fmt.Fprintln(cmd.OutOrStdout(), "Dry run complete; no action or operation record was created.")
	}
	return nil
}

func printDefaultRecord(cmd *cobra.Command, result defaultpkg.Result) {
	if result.RecordPath != "" {
		fmt.Fprintf(cmd.OutOrStdout(), "Operation record: %s\n", result.RecordPath)
	}
}

func recoverableDefaultRecord(record execution.Record) bool {
	if record.ID == "" || len(record.Steps) != 1 || record.Steps[0].Before == nil || record.Steps[0].After == nil {
		return false
	}
	return record.Steps[0].Before.Facts["default_alias_hash"] != "" &&
		record.Steps[0].Before.Facts["default_alias_hash"] != record.Steps[0].After.Facts["default_alias_hash"]
}

func newPlanCommand(deps commandDependencies) *cobra.Command {
	var toolID string
	var format string
	var online bool
	var projects []string
	var excludes []string
	var policyPath string
	command := &cobra.Command{
		Use:                   "plan",
		Short:                 "Preview a non-executable Node.js update Plan",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			options := planpkg.Options{ToolID: toolID, Format: planpkg.Format(format), Online: online, Projects: projects, Excludes: excludes, PolicyPath: policyPath}
			if err := planpkg.ValidateOptions(options); err != nil {
				return err
			}
			data, err := deps.generatePlan(cmd.Context(), options)
			if err != nil {
				return operationalError{err: fmt.Errorf("generate plan preview: %w", err)}
			}
			if _, err := cmd.OutOrStdout().Write(data); err != nil {
				return operationalError{err: fmt.Errorf("write plan preview: %w", err)}
			}
			return nil
		},
	}
	command.Flags().StringVar(&toolID, "tool", "", "tool ID; I13 supports runtime.node")
	command.Flags().StringVar(&format, "format", string(planpkg.FormatSummary), "output format: summary or json")
	command.Flags().BoolVar(&online, "online", false, "require fresh official version evidence")
	command.Flags().StringArrayVar(&projects, "project", nil, "scan an explicit project root (repeatable)")
	command.Flags().StringArrayVar(&excludes, "exclude", nil, "exclude a path below each project root (repeatable)")
	command.Flags().StringVar(&policyPath, "policy", "", "read an explicit EnvMason JSON policy file")
	return command
}

func newReportCommand(deps commandDependencies) *cobra.Command {
	var format string
	var categoryValues []string
	var severityValues []string
	var online bool
	var projects []string
	var excludes []string
	var policyPath string
	command := &cobra.Command{
		Use:                   "report",
		Short:                 "Generate a read-only macOS environment report",
		Args:                  cobra.NoArgs,
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			categories, err := report.ParseCategories(categoryValues)
			if err != nil {
				return err
			}
			severities, err := report.ParseSeverities(severityValues)
			if err != nil {
				return err
			}
			options := report.Options{Format: report.Format(format), Categories: categories, Severities: severities, Online: online, Projects: projects, Excludes: excludes, PolicyPath: policyPath}
			if err := report.ValidateOptions(options); err != nil {
				return err
			}
			data, err := deps.generateReport(cmd.Context(), options)
			if err != nil {
				return operationalError{err: fmt.Errorf("generate report: %w", err)}
			}
			if _, err := cmd.OutOrStdout().Write(data); err != nil {
				return operationalError{err: fmt.Errorf("write report: %w", err)}
			}
			return nil
		},
	}
	command.Flags().StringVar(&format, "format", string(report.FormatSummary), "output format: summary, markdown, or json")
	command.Flags().StringArrayVar(&categoryValues, "category", nil, "include a tool category (repeatable)")
	command.Flags().StringArrayVar(&severityValues, "severity", nil, "include a finding severity (repeatable)")
	command.Flags().BoolVar(&online, "online", false, "query official Node.js and Java version sources")
	command.Flags().StringArrayVar(&projects, "project", nil, "scan an explicit project root (repeatable)")
	command.Flags().StringArrayVar(&excludes, "exclude", nil, "exclude a path below each project root (repeatable)")
	command.Flags().StringVar(&policyPath, "policy", "", "read an explicit EnvMason JSON policy file")
	return command
}

func formatVersion(info buildinfo.Info) string {
	return fmt.Sprintf(
		"envmason %s\ncommit: %s\nbuilt: %s\ngo: %s\ntarget: %s\n",
		info.Version,
		info.Commit,
		info.BuildTime,
		info.GoVersion,
		info.Target,
	)
}
