package cli

import (
	"errors"
	"fmt"
	"os"

	"imux/internal/tui"

	"github.com/spf13/cobra"
)

// Execute runs the root command tree for imux.
// Default (no reserved first word) starts the TUI; argv is parsed for --tee,
// --log-filter, --name, and optional shell command lines. "run", "r", "help",
// and completion entrypoints go through Cobra.
func Execute() error {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "--version" {
		_, _ = fmt.Fprintln(os.Stdout, FormatVersion())
		return nil
	}
	if routeToCobraReserved(args) {
		cmd := NewRootCommand()
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	opts, err := ParseTUIModeArgs(args)
	if err != nil {
		if errors.Is(err, errTUIUsagePrinted) {
			return nil
		}
		return err
	}
	return tui.Run(opts)
}

// NewRootCommand creates the Cobra tree for batch mode ("run") and help.
func NewRootCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "imux",
		Short: "Interactive multiplexer; default command is the TUI (see imux -h)",
		Long: `Without a reserved first argument, imux starts the terminal UI. Optional
positional arguments are shell commands to run immediately in that session.

Use "imux run" for non-TUI execution (merged output, exits when children finish).`,
		Version:           FormatVersion(),
		SilenceUsage:      true,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown %q (try \"imux -h\" for the TUI, or \"imux help run\" for batch mode)", args[0])
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(NewRunCommand())
	cmd.InitDefaultHelpCmd()
	return cmd
}
