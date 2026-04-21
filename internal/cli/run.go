package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// RunOptions defines non-TUI execution parameters.
type RunOptions struct {
	Names      []string
	Grace      time.Duration
	NoFailFast bool
	TeePath    string
}

// NewRunCommand creates the non-interactive execution entrypoint.
func NewRunCommand() *cobra.Command {
	opts := RunOptions{}
	nameCSV := ""

	cmd := &cobra.Command{
		Use:     "run \"cmd1\" \"cmd2\" ...",
		Aliases: []string{"r"},
		Short:   "Run one or more commands without launching the TUI",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if nameCSV != "" {
				opts.Names = splitCSV(nameCSV)
			}
			return runNonTUI(cmd, opts, args)
		},
	}

	cmd.Flags().StringVar(&nameCSV, "name", "", "comma-separated process names (default: 1,2,3,...)")
	cmd.Flags().DurationVar(&opts.Grace, "grace", 60*time.Second, "grace period before forced kill after SIGTERM")
	cmd.Flags().BoolVar(&opts.NoFailFast, "no-fail-fast", false, "keep running when a process exits")
	cmd.Flags().StringVar(&opts.TeePath, "tee", "", "file path to tee merged output to (without ANSI colors)")

	return cmd
}

func runNonTUI(cmd *cobra.Command, opts RunOptions, commands []string) error {
	_ = opts
	_ = commands
	_, err := fmt.Fprintln(cmd.OutOrStdout(), "imux run: non-TUI mode scaffold (execution engine pending)")
	return err
}

func splitCSV(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
