package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayProcesses
	overlayInspector
	overlayHelp
)

type model struct {
	width        int
	height       int
	overlay      overlayKind
	helpReturnTo overlayKind // where Esc/? goes after closing help (if not overlayNone)
	processes    []string
	selected     int
	tick         int // bumped on tea.Tick so the log view keeps refreshing
}

type tickMsg time.Time

const tickInterval = 400 * time.Millisecond

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func newModel() model {
	return model{
		processes: []string{
			"api",
			"worker",
			"scheduler",
		},
		selected: 0,
	}
}

func (m model) Init() tea.Cmd {
	return tickCmd()
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.tick++
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "?":
			if m.overlay == overlayHelp {
				m.overlay = m.helpReturnTo
				m.helpReturnTo = overlayNone
			} else {
				m.helpReturnTo = m.overlay
				m.overlay = overlayHelp
			}
		case "esc":
			if m.overlay == overlayHelp {
				m.overlay = m.helpReturnTo
				m.helpReturnTo = overlayNone
			} else if m.overlay != overlayNone {
				m.overlay = overlayNone
			}
		case "p":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayProcesses {
				m.overlay = overlayNone
			} else {
				m.overlay = overlayProcesses
			}
		case "i":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			} else {
				m.overlay = overlayInspector
			}
		case "up", "k":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayProcesses && m.selected > 0 {
				m.selected--
			}
		case "down", "j":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayProcesses && m.selected < len(m.processes)-1 {
				m.selected++
			}
		case "enter":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayProcesses {
				m.overlay = overlayNone
			}
		case ",", "<":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayNone && m.selected > 0 {
				m.selected--
			}
		case ".", ">":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayNone && m.selected < len(m.processes)-1 {
				m.selected++
			}
		}
	}

	return m, nil
}

func (m model) View() string {
	if m.width < 40 || m.height < 8 {
		return "Terminal too small for imux TUI (need at least 40x8). Press q to quit."
	}

	footerH := 1
	bodyH := m.height - footerH
	if bodyH < 3 {
		bodyH = 3
	}

	body := m.renderBody(bodyH)
	footer := m.renderFooter()

	if m.overlay != overlayNone {
		modal := m.renderModal()
		body = compositeLogWithModal(body, modal, m.width, bodyH)
	}

	footerStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(footer)
	return lipgloss.JoinVertical(lipgloss.Left, body, footerStyled)
}

// renderFooter is one line of context-aware hints (truncated on narrow terminals).
func (m model) renderFooter() string {
	proc := m.processes[m.selected]
	var s string
	switch m.overlay {
	case overlayHelp:
		if m.helpReturnTo == overlayNone {
			s = "Help — Esc or ? closes. While viewing output: , . switch process; p list; i inspector; q quit."
		} else {
			s = "Help — Esc or ? returns to the previous panel. Same keys apply when you go back."
		}
	case overlayProcesses:
		s = "Processes — arrow keys or j/k to move, Enter picks, Esc closes. ? full help."
	case overlayInspector:
		s = fmt.Sprintf("Inspector for %s — metadata is placeholder for now. Esc closes. ? full help.", proc)
	default:
		s = fmt.Sprintf(
			"Watching %s — , or . previous/next process; p processes; i inspector; ? help; q quit.",
			proc,
		)
	}
	return padRight(truncate(s, m.width), m.width)
}

func (m model) renderBody(bodyH int) string {
	// Full-bleed log region (no border) — merged stdout/stderr placeholder until imux-38oz.
	w := m.width
	if w < 1 {
		w = 1
	}
	h := bodyH
	if h < 1 {
		h = 1
	}

	lines := m.placeholderStreamLines(h)
	for i := range lines {
		lines[i] = padRight(truncate(lines[i], w), w)
	}

	return strings.Join(lines, "\n")
}

func (m model) placeholderStreamLines(n int) []string {
	proc := m.processes[m.selected]
	t := m.tick
	out := make([]string, 0, n)
	out = append(out, fmt.Sprintf("[o] (%s) stdout line — multiplexed stream placeholder (t=%d)", proc, t))
	out = append(out, fmt.Sprintf("[e] (%s) stderr line — differentiated in imux-38oz (t=%d)", proc, t))
	for i := 2; i < n; i++ {
		if i%3 == 0 {
			out = append(out, fmt.Sprintf("[o] (%s) line=%d tick=%d …", proc, i, t+i))
		} else {
			out = append(out, fmt.Sprintf("[e] (%s) line=%d tick=%d …", proc, i, t+i))
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	for len(out) < n {
		out = append(out, "")
	}
	return out
}

func (m model) renderModal() string {
	maxW := min(56, m.width-6)
	maxH := min(16, m.height-6)
	if m.overlay == overlayHelp {
		maxW = min(72, m.width-4)
		maxH = min(22, m.height-4)
	}
	if maxW < 24 {
		maxW = 24
	}
	if maxH < 7 {
		maxH = 7
	}

	innerW := maxW - 2
	innerLines := maxH - 2 // lines inside border (title + body rows)
	if innerW < 4 {
		innerW = 4
	}
	if innerLines < 3 {
		innerLines = 3
	}

	var title string
	var bodyLines []string
	switch m.overlay {
	case overlayHelp:
		title = "Help"
		proc := m.processes[m.selected]
		bodyLines = []string{
			"One merged log view; focus picks which process labels the",
			"stream (placeholder until live attach).",
			"",
			"Keys:",
			"  , or .     previous / next process (main view)",
			"  p          process list (pick focus)",
			"  i          inspector for focused process",
			"  ?          this help (again or Esc closes)",
			"  Esc        close top overlay",
			"  q Ctrl+c   quit",
			"",
			fmt.Sprintf("Focus: %s", proc),
		}
	case overlayProcesses:
		title = "Processes"
		for i, name := range m.processes {
			prefix := "  "
			if i == m.selected {
				prefix = "> "
			}
			bodyLines = append(bodyLines, prefix+name)
		}
		bodyLines = append(bodyLines, "", "Enter confirms focus · Esc closes · ? help")
	case overlayInspector:
		title = "Inspector"
		proc := m.processes[m.selected]
		bodyLines = []string{
			fmt.Sprintf("Process: %s", proc),
			"Metadata / state placeholder (imux-1xp2).",
			"",
			"Esc closes · ? help",
		}
	default:
		title = ""
	}

	bodyRows := innerLines - 1 // first inner row is title bar
	if bodyRows < 1 {
		bodyRows = 1
	}
	for len(bodyLines) < bodyRows {
		bodyLines = append(bodyLines, "")
	}
	if len(bodyLines) > bodyRows {
		bodyLines = bodyLines[:bodyRows]
	}
	for i := range bodyLines {
		bodyLines[i] = padRight(truncate(bodyLines[i], innerW), innerW)
	}

	header := padRight(truncate(" "+title+" ", innerW), innerW)
	all := append([]string{header}, bodyLines...)

	style := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("12")).
		Background(lipgloss.Color("235")).
		Foreground(lipgloss.Color("252")).
		Width(innerW)

	return style.Render(strings.Join(all, "\n"))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}
	rs := []rune(s)
	if len(rs) >= maxWidth {
		return string(rs[:maxWidth-1]) + "…"
	}
	return s
}

func padRight(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

// Run launches the alt-screen Bubble Tea application.
func Run() error {
	p := tea.NewProgram(newModel(), tea.WithAltScreen())
	_, err := p.Run()
	return err
}
