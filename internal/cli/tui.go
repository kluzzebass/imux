package cli

import (
	"github.com/spf13/cobra"
	"imux/internal/tui"
)

// NewTUICommand creates the interactive entrypoint.
func NewTUICommand() *cobra.Command {
	return &cobra.Command{
		Use:     "tui",
		Aliases: []string{"t"},
		Short:   "Launch interactive TUI mode",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return tui.Run()
		},
	}
}
