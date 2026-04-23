package tui

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
	"imux/internal/core"
)

// supOpDoneMsg is sent after a single supervisor call finishes off the UI thread.
type supOpDoneMsg struct {
	op   string
	id   core.ProcessID
	name string
	err  error
}

// supShutdownDoneMsg is sent after graceful shutdown of all children (quit path).
type supShutdownDoneMsg struct{}

// supReplaceSaveDoneMsg is sent after async stop+replace during edit save.
type supReplaceSaveDoneMsg struct {
	id         core.ProcessID
	name       string
	err        error // stop failure
	replaceErr error // replace failure after a successful stop
}

func supStopCmd(sup *core.ExecSupervisor, id core.ProcessID, displayName string) tea.Cmd {
	if sup == nil {
		return nil
	}
	return func() tea.Msg {
		err := sup.Stop(context.Background(), id)
		return supOpDoneMsg{op: "stop", id: id, name: displayName, err: err}
	}
}

func supSignalCmd(sup *core.ExecSupervisor, id core.ProcessID, displayName string, choice core.UserSignal) tea.Cmd {
	if sup == nil {
		return nil
	}
	return func() tea.Msg {
		err := sup.SendUserSignal(context.Background(), id, choice)
		return supOpDoneMsg{op: "signal", id: id, name: displayName, err: err}
	}
}

func supRestartCmd(sup *core.ExecSupervisor, id core.ProcessID, displayName string) tea.Cmd {
	if sup == nil {
		return nil
	}
	return func() tea.Msg {
		err := sup.Restart(context.Background(), id)
		return supOpDoneMsg{op: "restart", id: id, name: displayName, err: err}
	}
}

func supStartCmd(sup *core.ExecSupervisor, id core.ProcessID, displayName string) tea.Cmd {
	if sup == nil {
		return nil
	}
	return func() tea.Msg {
		err := sup.Start(context.Background(), id)
		return supOpDoneMsg{op: "start", id: id, name: displayName, err: err}
	}
}

func supPauseCmd(sup *core.ExecSupervisor, id core.ProcessID, displayName string) tea.Cmd {
	if sup == nil {
		return nil
	}
	return func() tea.Msg {
		err := sup.Pause(context.Background(), id)
		return supOpDoneMsg{op: "pause", id: id, name: displayName, err: err}
	}
}

func supContinueCmd(sup *core.ExecSupervisor, id core.ProcessID, displayName string) tea.Cmd {
	if sup == nil {
		return nil
	}
	return func() tea.Msg {
		err := sup.Continue(context.Background(), id)
		return supOpDoneMsg{op: "continue", id: id, name: displayName, err: err}
	}
}

// supShutdownAllCmd stops every listed id sequentially in a worker (UI stays responsive).
func supShutdownAllCmd(sup *core.ExecSupervisor, ids []core.ProcessID) tea.Cmd {
	if sup == nil {
		return func() tea.Msg { return supShutdownDoneMsg{} }
	}
	ids = append([]core.ProcessID(nil), ids...)
	return func() tea.Msg {
		ctx := context.Background()
		for _, id := range ids {
			_ = sup.Stop(ctx, id)
		}
		return supShutdownDoneMsg{}
	}
}

// supStopThenReplaceSpecCmd stops a running child then applies ReplaceSpec (both in a worker).
func supStopThenReplaceSpecCmd(sup *core.ExecSupervisor, id core.ProcessID, displayName string, spec core.ProcessSpec) tea.Cmd {
	if sup == nil {
		return nil
	}
	return func() tea.Msg {
		ctx := context.Background()
		if err := sup.Stop(ctx, id); err != nil {
			return supReplaceSaveDoneMsg{id: id, name: displayName, err: err}
		}
		if err := sup.ReplaceSpec(ctx, id, spec); err != nil {
			return supReplaceSaveDoneMsg{id: id, name: displayName, replaceErr: err}
		}
		return supReplaceSaveDoneMsg{id: id, name: displayName}
	}
}
