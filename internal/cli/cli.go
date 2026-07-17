package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/gitbagHero/EnvMason/internal/buildinfo"
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
	return execute(args, stdout, stderr, info, commandDependencies{generateReport: report.Generate, generatePlan: planpkg.Generate})
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
	generateReport func(context.Context, report.Options) ([]byte, error)
	generatePlan   func(context.Context, planpkg.Options) ([]byte, error)
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

	return root
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
