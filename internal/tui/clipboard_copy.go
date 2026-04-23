package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/atotto/clipboard"
)

// copyLogDoneMsg is sent after an async attempt to write plain text to the system clipboard.
type copyLogDoneMsg struct {
	err error
}

func runCopyLogPlainCmd(text string) tea.Cmd {
	return func() tea.Msg {
		return copyLogDoneMsg{err: clipboard.WriteAll(text)}
	}
}
