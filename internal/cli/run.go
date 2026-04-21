package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"imux/internal/core"

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
	names := opts.Names
	if len(names) == 0 {
		for i := range commands {
			names = append(names, strconv.Itoa(i+1))
		}
	}
	if len(names) != len(commands) {
		return fmt.Errorf("%d names but %d commands", len(names), len(commands))
	}

	bus := core.NewChanEventBus()
	store := core.NewMapStateStore()
	sup := core.NewExecSupervisor(bus, store)
	sup.SetStopGrace(opts.Grace)

	var teeOut *os.File
	if opts.TeePath != "" {
		f, err := os.OpenFile(opts.TeePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return fmt.Errorf("tee %q: %w", opts.TeePath, err)
		}
		defer f.Close()
		teeOut = f
	}

	errOut := cmd.ErrOrStderr()
	out := cmd.OutOrStdout()
	sub := bus.Subscribe(4096)
	go func() {
		for e := range sub {
			switch e.Type {
			case core.EventProcessOutput:
				tag := e.Stream
				if tag == "" {
					tag = "?"
				}
				who := string(e.ProcessID)
				if e.ProcessName != "" {
					who = e.ProcessName
				}
				line := fmt.Sprintf("[%s|%s] %s\n", tag, who, e.Message)
				_, _ = fmt.Fprint(out, line)
				if teeOut != nil {
					_, _ = fmt.Fprint(teeOut, line)
				}
			case core.EventProcessError:
				_, _ = fmt.Fprintf(errOut, "imux error: %s [%s] %s\n", e.ProcessID, e.Type, e.Message)
			default:
				_, _ = fmt.Fprintf(errOut, "imux: [%s] %s %s\n", e.Type, e.ProcessID, e.Message)
			}
		}
	}()

	ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shCmd, shArg := shellInvocation()
	ids := make([]core.ProcessID, len(commands))
	for i, c := range commands {
		id := core.ProcessID(names[i])
		ids[i] = id
		args := []string{shArg, c}
		if err := sup.Register(ctx, core.ProcessSpec{
			ID:      id,
			Name:    names[i],
			Command: shCmd,
			Args:    args,
			Restart: core.RestartConfig{Policy: core.RestartNever},
		}); err != nil {
			return err
		}
		if err := sup.Start(ctx, id); err != nil {
			return err
		}
	}

	failFastCh := make(chan core.ProcessID, 1)
	if !opts.NoFailFast {
		go func() {
			sub2 := bus.Subscribe(128)
			for e := range sub2 {
				if e.Type == core.EventProcessFailed {
					select {
					case failFastCh <- e.ProcessID:
					default:
					}
				}
			}
		}()
	}

	waitAll := func() {
		deadline := time.Now().Add(2 * time.Hour)
		for time.Now().Before(deadline) {
			allTerminal := true
			snap := store.Snapshot()
			for _, id := range ids {
				st, ok := snap.Processes[id]
				if !ok || (st != core.StateExited && st != core.StateFailed) {
					allTerminal = false
					break
				}
			}
			if allTerminal {
				return
			}
			time.Sleep(40 * time.Millisecond)
		}
	}

	go func() {
		<-ctx.Done()
		for i := len(ids) - 1; i >= 0; i-- {
			id := ids[i]
			st, ok := store.Get(id)
			if !ok {
				continue
			}
			if st == core.StateRunning || st == core.StatePaused || st == core.StateStarting {
				_ = sup.Stop(context.Background(), id)
			}
		}
	}()

	if opts.NoFailFast {
		waitAll()
	} else {
		waitDone := make(chan struct{})
		go func() {
			waitAll()
			close(waitDone)
		}()
		select {
		case id := <-failFastCh:
			_, _ = fmt.Fprintf(errOut, "imux: process %s failed; stopping others\n", id)
			stop()
			<-waitDone
		case <-waitDone:
		}
	}

	exitCode := 0
	snap := store.Snapshot()
	for _, id := range ids {
		if st, ok := snap.Processes[id]; ok && st == core.StateFailed {
			exitCode = 1
		}
	}
	if exitCode != 0 {
		stop()
		for i := len(ids) - 1; i >= 0; i-- {
			id := ids[i]
			st, ok := store.Get(id)
			if !ok {
				continue
			}
			if st == core.StateRunning || st == core.StatePaused || st == core.StateStarting {
				_ = sup.Stop(context.Background(), id)
			}
		}
		os.Exit(exitCode)
	}
	return nil
}

func shellInvocation() (cmd string, arg string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", "/C"
	}
	return "sh", "-c"
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
