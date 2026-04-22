package tui

import (
	"fmt"
	"runtime"

	"imux/internal/core"

	"github.com/charmbracelet/lipgloss"
)

type killMenuChoice struct {
	label string
	sig   core.UserSignal
}

func killSignalMenu() []killMenuChoice {
	if runtime.GOOS == "windows" {
		return []killMenuChoice{
			{label: "Graceful stop (wait, then hard terminate if needed)", sig: core.UserSignalStopGraceful},
			{label: "Force terminate (hard kill)", sig: core.UserSignalForceKill},
		}
	}
	return []killMenuChoice{
		{label: "Graceful stop (SIGTERM, wait, SIGKILL if still alive)", sig: core.UserSignalStopGraceful},
		{label: "SIGTERM — single signal (no automatic follow-up kill)", sig: core.UserSignalSIGTERMOnce},
		{label: "SIGINT — interrupt", sig: core.UserSignalInterrupt},
		{label: "SIGHUP — hang up", sig: core.UserSignalHangup},
		{label: "SIGQUIT — quit (often core dump)", sig: core.UserSignalSIGQUIT},
		{label: "SIGUSR1 — user-defined 1", sig: core.UserSignalSIGUSR1},
		{label: "SIGUSR2 — user-defined 2", sig: core.UserSignalSIGUSR2},
		{label: "SIGPIPE — broken pipe", sig: core.UserSignalSIGPIPE},
		{label: "SIGWINCH — window size changed", sig: core.UserSignalSIGWINCH},
		{label: "SIGALRM — alarm clock", sig: core.UserSignalSIGALRM},
		{label: "SIGABRT — abort", sig: core.UserSignalSIGABRT},
		{label: "SIGKILL — cannot be caught", sig: core.UserSignalForceKill},
	}
}

func killSignalModalLines(sel int, bulk bool, id core.ProcessID, displayName string, bulkCount int) []string {
	menu := killSignalMenu()
	if sel < 0 {
		sel = 0
	}
	if len(menu) == 0 {
		return []string{"(no signal choices)"}
	}
	if sel >= len(menu) {
		sel = len(menu) - 1
	}
	hl := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Bold(true)
	normal := lipgloss.NewStyle().Foreground(lipgloss.Color("247"))
	var head []string
	if bulk {
		head = []string{
			fmt.Sprintf("Target: all %d running process(es)", bulkCount),
			"",
			"↑↓ move · Enter send · Esc cancel",
			"",
		}
	} else {
		head = []string{
			fmt.Sprintf("Target: %s (%s)", displayName, id),
			"",
			"↑↓ move · Enter send · Esc cancel",
			"",
		}
	}
	lines := append([]string(nil), head...)
	for i, ch := range menu {
		prefix := "  "
		if i == sel {
			lines = append(lines, hl.Render("▸ "+ch.label))
		} else {
			lines = append(lines, normal.Render(prefix+ch.label))
		}
	}
	return lines
}
