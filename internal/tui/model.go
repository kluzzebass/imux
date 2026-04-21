package tui

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"imux/internal/core"

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
	helpReturnTo overlayKind
	processes    []string
	ids          []core.ProcessID
	selected     int
	tick         int

	sup    *core.ExecSupervisor
	store  core.StateStore
	bus    core.EventBus
	sub    <-chan core.Event
	events []string
}

type tickMsg time.Time

const tickInterval = 400 * time.Millisecond

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func demoLongCommand() (cmd string, args []string) {
	if runtime.GOOS == "windows" {
		return "timeout", []string{"/t", "86400", "/nobreak"}
	}
	return "sleep", []string{"86400"}
}

func newModel() *model {
	bus := core.NewChanEventBus()
	store := core.NewMapStateStore()
	sup := core.NewExecSupervisor(bus, store)
	sup.SetStopGrace(10 * time.Second)
	ctx := context.Background()

	demos := []struct {
		id, name string
	}{
		{"a", "demo-a"},
		{"b", "demo-b"},
		{"c", "demo-c"},
	}
	shCmd, shArg := "sh", "-c"
	if runtime.GOOS == "windows" {
		shCmd, shArg = "cmd.exe", "/C"
	}
	dc, da := demoLongCommand()
	inner := fmt.Sprintf("%s %s", dc, strings.Join(da, " "))
	if runtime.GOOS == "windows" {
		inner = fmt.Sprintf("%s %s", dc, strings.Join(da, " "))
	}

	names := make([]string, 0, len(demos))
	ids := make([]core.ProcessID, 0, len(demos))
	for _, d := range demos {
		id := core.ProcessID(d.id)
		args := []string{shArg, inner}
		if runtime.GOOS == "windows" {
			args = []string{shArg, inner}
		}
		_ = sup.Register(ctx, core.ProcessSpec{
			ID:      id,
			Name:    d.name,
			Command: shCmd,
			Args:    args,
			Restart: core.RestartConfig{Policy: core.RestartNever},
		})
		names = append(names, d.name)
		ids = append(ids, id)
	}

	return &model{
		sup:       sup,
		store:     store,
		bus:       bus,
		sub:       bus.Subscribe(512),
		processes: names,
		ids:       ids,
		selected:  0,
		events:    []string{"[o] (imux) lifecycle log — start processes with s; errors appear here and on stderr from imux run."},
	}
}

func (m *model) currentID() core.ProcessID {
	if m.selected < 0 || m.selected >= len(m.ids) {
		return ""
	}
	return m.ids[m.selected]
}

func (m *model) currentName() string {
	if m.selected < 0 || m.selected >= len(m.processes) {
		return ""
	}
	return m.processes[m.selected]
}

func (m *model) appendLogLine(line string) {
	m.events = append(m.events, line)
	if len(m.events) > 500 {
		m.events = m.events[len(m.events)-500:]
	}
}

func (m *model) appendCtlErr(op string, err error) {
	if err == nil {
		m.appendLogLine(fmt.Sprintf("[ok] %s %s (%s)", op, m.currentName(), m.currentID()))
		return
	}
	m.appendLogLine(fmt.Sprintf("[error] %s %s (%s): %v", op, m.currentName(), m.currentID(), err))
}

func (m *model) drainEvents() {
	if m.sub == nil {
		return
	}
	for {
		select {
		case e := <-m.sub:
			if e.Type == core.EventProcessOutput {
				tag := e.Stream
				if tag == "" {
					tag = "?"
				}
				who := string(e.ProcessID)
				if e.ProcessName != "" {
					who = e.ProcessName
				}
				m.appendLogLine(fmt.Sprintf("[%s|%s] %s", tag, who, e.Message))
				continue
			}
			m.appendLogLine(fmt.Sprintf("[%s] %s %s", e.Type, e.ProcessID, e.Message))
		default:
			return
		}
	}
}

func (m *model) refreshProcs() {
	if m.sup == nil {
		return
	}
	specs, err := m.sup.List(context.Background())
	if err != nil {
		return
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	names := make([]string, len(specs))
	ids := make([]core.ProcessID, len(specs))
	for i, sp := range specs {
		names[i] = sp.Name
		ids[i] = sp.ID
	}
	m.processes = names
	m.ids = ids
	if m.selected >= len(m.ids) {
		m.selected = len(m.ids) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *model) shutdownProcs() {
	if m.sup == nil {
		return
	}
	ctx := context.Background()
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok {
			continue
		}
		if st == core.StateRunning || st == core.StatePaused || st == core.StateStarting {
			_ = m.sup.Stop(ctx, id)
		}
	}
}

func (m *model) Init() tea.Cmd {
	return tickCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		m.tick++
		m.drainEvents()
		m.refreshProcs()
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.shutdownProcs()
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
		case "up":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayProcesses && m.selected > 0 {
				m.selected--
			}
		case "k":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayProcesses && m.selected > 0 {
				m.selected--
				break
			}
			if m.overlay == overlayNone && m.sup != nil {
				ctx := context.Background()
				if id := m.currentID(); id != "" {
					m.appendCtlErr("kill", m.sup.Kill(ctx, id))
				}
			}
		case "down":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayProcesses && m.selected < len(m.processes)-1 {
				m.selected++
			}
		case "j":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayProcesses && m.selected < len(m.processes)-1 {
				m.selected++
				break
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
		default:
			if m.overlay == overlayNone && m.sup != nil {
				ctx := context.Background()
				id := m.currentID()
				if id == "" {
					break
				}
				switch msg.String() {
				case "s":
					m.appendCtlErr("start", m.sup.Start(ctx, id))
				case "t":
					m.appendCtlErr("stop", m.sup.Stop(ctx, id))
				case "z":
					m.appendCtlErr("pause", m.sup.Pause(ctx, id))
				case "v":
					m.appendCtlErr("continue", m.sup.Continue(ctx, id))
				case "y":
					m.appendCtlErr("restart", m.sup.Restart(ctx, id))
				}
			}
		}
	}

	return m, nil
}

func (m *model) View() string {
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

func (m *model) renderFooter() string {
	proc := m.currentName()
	var s string
	switch m.overlay {
	case overlayHelp:
		if m.helpReturnTo == overlayNone {
			s = "Help — Esc or ? closes. s start · t stop · k kill · z pause · v continue · y restart · p list · i inspector · , . focus · q quit."
		} else {
			s = "Help — Esc or ? returns to the previous panel."
		}
	case overlayProcesses:
		s = "Processes — j/k move, Enter closes. ? help."
	case overlayInspector:
		s = fmt.Sprintf("Inspector — %s · Esc closes · ? help.", proc)
	default:
		s = fmt.Sprintf(
			"%s — s start t stop k kill z pause v continue y restart · p i ? · q quit",
			proc,
		)
	}
	return padRight(truncate(s, m.width), m.width)
}

func (m *model) renderBody(bodyH int) string {
	w := m.width
	if w < 1 {
		w = 1
	}
	h := bodyH
	if h < 1 {
		h = 1
	}

	lines := m.composeLines(h)
	for i := range lines {
		lines[i] = padRight(truncate(lines[i], w), w)
	}

	return strings.Join(lines, "\n")
}

func (m *model) composeLines(n int) []string {
	ev := m.events
	if len(ev) >= n {
		return append([]string(nil), ev[len(ev)-n:]...)
	}
	ph := m.placeholderStreamLines(n - len(ev))
	out := append([]string(nil), ph...)
	out = append(out, ev...)
	for len(out) < n {
		out = append(out, "")
	}
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return out
}

func (m *model) placeholderStreamLines(n int) []string {
	proc := m.currentName()
	if proc == "" {
		proc = "(none)"
	}
	t := m.tick
	out := make([]string, 0, n)
	out = append(out, fmt.Sprintf("[o] (%s) stdout placeholder (t=%d)", proc, t))
	out = append(out, fmt.Sprintf("[e] (%s) stderr placeholder (t=%d)", proc, t))
	for i := 2; i < n; i++ {
		if i%3 == 0 {
			out = append(out, fmt.Sprintf("[o] (%s) line=%d …", proc, i))
		} else {
			out = append(out, fmt.Sprintf("[e] (%s) line=%d …", proc, i))
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

func (m *model) renderModal() string {
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
	innerLines := maxH - 2
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
		proc := m.currentName()
		bodyLines = []string{
			"One merged log view; lifecycle lines come from the supervisor",
			"event bus (errors are prefixed with [error]).",
			"",
			"Keys:",
			"  s t k z v y   start / stop / kill / pause / continue / restart",
			"  , or .        previous / next process",
			"  p i           process list · inspector",
			"  ? Esc         help · close overlay",
			"  q Ctrl+c      quit (stops running demos)",
			"",
			fmt.Sprintf("Focus: %s (%s)", proc, m.currentID()),
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
		bodyLines = append(bodyLines, "", "Enter closes · ? help")
	case overlayInspector:
		title = "Inspector"
		id := m.currentID()
		st, ok := m.store.Get(id)
		stStr := string(st)
		if !ok {
			stStr = "(unknown)"
		}
		bodyLines = []string{
			fmt.Sprintf("Process: %s (%s)", m.currentName(), id),
			fmt.Sprintf("State: %s", stStr),
			"",
			"Esc closes · ? help",
		}
	default:
		title = ""
	}

	bodyRows := innerLines - 1
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
