package cli

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"imux/internal/tui"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// errTUIUsagePrinted is returned after -h/--help so Execute can exit 0 without starting the TUI.
var errTUIUsagePrinted = errors.New("tui usage printed")

// routeToCobraReserved reports argv that must be handled by the Cobra tree
// (run / help / shell completion), not as TUI bootstrap.
func routeToCobraReserved(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "run", "r", "help", "completion":
		return true
	default:
		// e.g. __complete, __completeNoDesc
		return strings.HasPrefix(args[0], cobra.ShellCompRequestCmd)
	}
}

// ParseTUIModeArgs parses argv for the default interactive imux command (TUI).
// It accepts the same core flags as before (--tee, --log-filter) plus optional
// --name and one or more shell command lines, which become running processes when the TUI starts.
func ParseTUIModeArgs(args []string) (tui.Options, error) {
	var opts tui.Options
	fs := pflag.NewFlagSet("imux", pflag.ContinueOnError)
	fs.SetInterspersed(false)
	fs.Usage = func() {
		_, _ = fmt.Fprint(os.Stderr, tuiUsageText)
	}
	var nameCSV string
	fs.StringVar(&opts.TeePath, "tee", "", "append merged plain-text log lines (no ANSI) to this file path")
	fs.StringVar(&opts.LogFilter, "log-filter", "", "initial log filter: re:PAT or bare Go regexp (empty = none)")
	fs.StringVar(&nameCSV, "name", "", "comma-separated display names for the following shell commands (default: 1,2,3,...)")
	var showHelp bool
	fs.BoolVarP(&showHelp, "help", "h", false, "show help")
	if err := fs.Parse(args); err != nil {
		return opts, err
	}
	if showHelp {
		_, _ = fmt.Fprint(os.Stdout, tuiUsageText)
		return opts, errTUIUsagePrinted
	}
	commands := fs.Args()
	if nameCSV != "" && len(commands) == 0 {
		return opts, fmt.Errorf("--name requires at least one shell command after the flags")
	}
	if len(commands) == 0 {
		return opts, nil
	}
	names := SplitCSV(nameCSV)
	if len(names) == 0 {
		for i := range commands {
			names = append(names, strconv.Itoa(i+1))
		}
	}
	if len(names) != len(commands) {
		return opts, fmt.Errorf("%d names in --name but %d commands (each positional is one shell command)", len(names), len(commands))
	}
	if err := DuplicateSlotIDs(names); err != nil {
		return opts, fmt.Errorf("--name: %w", err)
	}
	for i, line := range commands {
		opts.Bootstrap = append(opts.Bootstrap, tui.BootstrapProc{
			ID:   names[i],
			Line: line,
		})
	}
	return opts, nil
}

const tuiUsageText = `imux — interactive multiplexer (terminal UI)

Usage:
  imux [flags]
  imux [flags] [--name n1,n2,...] <cmd> [<cmd> ...]

Start the TUI. With one or more <cmd> arguments (each a full shell command line),
registers and starts those processes before the UI appears (same idea as multirun).

Flags:
  --tee path          append merged plain-text log lines (no ANSI) to this file
  --log-filter spec   initial log filter (re:… or bare Go regexp)
  --name csv          comma-separated names for the commands (default 1,2,3,…)
  -h, --help          show this help

Non-interactive mode (no TUI, merged output on stdout, exits when children finish):
  imux run [flags] <cmd> [<cmd> ...]
  imux run --help

`
