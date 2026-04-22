package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

func newNameLineTI() textinput.Model {
	t := textinput.New()
	t.Prompt = "> Name: "
	t.Placeholder = ""
	t.CharLimit = 256
	t.ShowSuggestions = false
	t.Cursor.Style = lipgloss.NewStyle()
	return t
}

func newCmdLineTI() textinput.Model {
	t := textinput.New()
	t.Prompt = "$ "
	t.Placeholder = ""
	t.CharLimit = 4000
	t.ShowSuggestions = false
	t.Cursor.Style = lipgloss.NewStyle()
	return t
}

func newFilterPatternTI() textinput.Model {
	t := textinput.New()
	t.Prompt = "> "
	t.Placeholder = ""
	t.CharLimit = 512
	t.ShowSuggestions = false
	t.Cursor.Style = lipgloss.NewStyle()
	return t
}

func (m *model) syncFilterInpWidth(innerW int) {
	m.filterInp.Width = lineFormTextWidth(innerW, m.filterInp)
}

// lineFormInnerW matches renderModal inner width for add/edit overlays.
func (m *model) lineFormInnerW() int {
	maxW := min(72, m.width-4)
	if maxW < 24 {
		maxW = 24
	}
	innerW := maxW - 2
	if innerW < 4 {
		innerW = 4
	}
	return innerW
}

func lineFormTextWidth(innerW int, ti textinput.Model) int {
	pw := lipgloss.Width(ti.Prompt)
	w := innerW - pw
	if w < 1 {
		w = 1
	}
	return w
}

func (m *model) syncLineFormWidths(innerW int) {
	m.addNameInp.Width = lineFormTextWidth(innerW, m.addNameInp)
	m.addCmdInp.Width = lineFormTextWidth(innerW, m.addCmdInp)
	m.editNameInp.Width = lineFormTextWidth(innerW, m.editNameInp)
	m.editCmdInp.Width = lineFormTextWidth(innerW, m.editCmdInp)
}

func (m *model) blurAllLineInputs() {
	m.addNameInp.Blur()
	m.addCmdInp.Blur()
	m.editNameInp.Blur()
	m.editCmdInp.Blur()
}

// refocusLineFormCurrent blurs all line inputs then focuses the active field for the current overlay.
func (m *model) refocusLineFormCurrent() tea.Cmd {
	m.blurAllLineInputs()
	if m.overlay == overlayEditProcess {
		if m.lineOverlayField == lineFormNameField {
			return m.editNameInp.Focus()
		}
		return m.editCmdInp.Focus()
	}
	if m.overlay == overlayAddProcess {
		if m.lineOverlayField == lineFormNameField {
			return m.addNameInp.Focus()
		}
		return m.addCmdInp.Focus()
	}
	return nil
}

func (m *model) lineFormModalLines(innerW int, edit bool, footer string) []string {
	m.syncLineFormWidths(innerW)
	var nameTI, cmdTI *textinput.Model
	if edit {
		nameTI, cmdTI = &m.editNameInp, &m.editCmdInp
	} else {
		nameTI, cmdTI = &m.addNameInp, &m.addCmdInp
	}
	nameView := nameTI.View()
	if m.nameEntryConflicts(edit) {
		nameView = lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(nameView)
	}
	return []string{
		nameView,
		cmdTI.View(),
		"",
		footer,
	}
}

func (m *model) dispatchLineFormUpdate(msg tea.Msg, edit bool) tea.Cmd {
	var ti *textinput.Model
	if edit {
		if m.lineOverlayField == lineFormNameField {
			ti = &m.editNameInp
		} else {
			ti = &m.editCmdInp
		}
	} else {
		if m.lineOverlayField == lineFormNameField {
			ti = &m.addNameInp
		} else {
			ti = &m.addCmdInp
		}
	}
	var cmd tea.Cmd
	*ti, cmd = ti.Update(msg)
	return cmd
}
