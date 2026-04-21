package cli

import (
	"github.com/spf13/cobra"
)

// Execute runs the root command tree for imux.
func Execute() error {
	return NewRootCommand().Execute()
}

// NewRootCommand creates the top-level CLI command with both
// interactive and non-interactive entrypoints.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "imux",
		Short:             "Interactive multiplexer with optional non-TUI execution",
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	}

	cmd.AddCommand(
		NewRunCommand(),
		NewTUICommand(),
	)

	return cmd
}
