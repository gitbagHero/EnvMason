package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/gitbagHero/EnvMason/internal/buildinfo"
)

const (
	ExitSuccess = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Execute runs the EnvMason CLI with explicit inputs and outputs so command
// behavior can be tested without changing process-global state.
func Execute(args []string, stdout, stderr io.Writer, info buildinfo.Info) int {
	root := newRootCommand(info, stdout, stderr)
	root.SetArgs(args)

	if err := root.Execute(); err != nil {
		fmt.Fprintf(stderr, "Error: %v\n", err)
		fmt.Fprintln(stderr, "Run 'envmason help' for usage.")
		return ExitUsage
	}

	return ExitSuccess
}

func newRootCommand(info buildinfo.Info, stdout, stderr io.Writer) *cobra.Command {
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

	return root
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
