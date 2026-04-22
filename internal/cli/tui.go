package cli

import (
	"imux/internal/tui"

	"github.com/spf13/cobra"
)

// NewTUICommand creates the interactive entrypoint.
func NewTUICommand() *cobra.Command {
	opts := tui.Options{}
	cmd := &cobra.Command{
		Use:     "tui",
		Aliases: []string{"t"},
		Short:   "Launch interactive TUI mode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return tui.Run(opts)
		},
	}
	cmd.Flags().StringVar(&opts.TeePath, "tee", "", "append merged plain-text log lines (no ANSI) to this file path")
	cmd.Flags().StringVar(&opts.LogFilter, "log-filter", "", "initial log filter: glob:PAT, re:PAT, or bare glob pattern")
	return cmd
}
