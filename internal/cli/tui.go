package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// NewTUICommand creates the interactive entrypoint.
func NewTUICommand() *cobra.Command {
	return &cobra.Command{
		Use:     "tui",
		Aliases: []string{"t"},
		Short:   "Launch interactive TUI mode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), "imux tui: interactive mode scaffold (renderer pending)")
			return err
		},
	}
}
