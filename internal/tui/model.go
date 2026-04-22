package tui

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"time"

	"imux/internal/core"
	"imux/internal/inspect"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayInspector
	overlayHelp
	overlayAddProcess
	overlayEditProcess
)

type model struct {
	width        int
	height       int
	overlay      overlayKind
	helpReturnTo overlayKind
	processes    []string // display names from ProcessSpec
	dockCmd      []string // one-line shell command for dock (Command + Args)
	ids          []core.ProcessID
	selected     int
	dockScroll   int // first visible row index when len(ids) > dock capacity
	tick         int

	sup    *core.ExecSupervisor
	store  core.StateStore
	bus    core.EventBus
	sub    <-chan core.Event
	events []string

	inspectLines   []string
	inspectCPU     *inspect.CPUSample
	inspectFocusID core.ProcessID

	addBuf           string
	addNameBuf       string
	nextUserSeq      int
	editBuf          string
	editNameBuf      string
	editTargetID     core.ProcessID
	lineOverlayField int // lineFormNameField or lineFormCmdField
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

func formatExecLine(cmd string, args []string) string {
	if len(args) == 0 {
		return cmd
	}
	return cmd + " " + strings.Join(args, " ")
}

func shellWrapUserCommand(script string) (cmd string, args []string) {
	if runtime.GOOS == "windows" {
		return "cmd.exe", []string{"/C", script}
	}
	return "sh", []string{"-c", script}
}

// innerCommandForEdit returns the user-editable script, stripping one imux-style
// sh -c / cmd /C wrapper so saving does not nest shells.
func innerCommandForEdit(sp core.ProcessSpec) string {
	if runtime.GOOS == "windows" {
		if strings.EqualFold(sp.Command, "cmd.exe") && len(sp.Args) >= 2 && strings.EqualFold(sp.Args[0], "/c") {
			return strings.Join(sp.Args[1:], " ")
		}
	} else {
		if sp.Command == "sh" && len(sp.Args) >= 2 && sp.Args[0] == "-c" {
			return strings.Join(sp.Args[1:], " ")
		}
	}
	return formatExecLine(sp.Command, sp.Args)
}

func trimLastRune(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return string(r[:len(r)-1])
}

func nameFromCommandLine(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		n := strings.TrimSpace(line)
		if n == "" {
			return "proc"
		}
		return truncate(n, 32)
	}
	return truncate(fields[0], 32)
}

// lineFormNameField / lineFormCmdField are focused buffers in add/edit overlays.
const (
	lineFormNameField = 0
	lineFormCmdField  = 1
)

func sanitizeDisplayName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	return truncate(s, 48)
}

// prefixedInputLine draws prefix + truncated buffer + optional blinking caret for the active field.
func prefixedInputLine(prefix, buf string, innerW int, active bool, tick int) string {
	pw := lipgloss.Width(prefix)
	maxBuf := innerW - pw
	if maxBuf < 1 {
		return truncate(prefix, innerW)
	}
	if !active {
		return prefix + truncate(buf, maxBuf)
	}
	caret := "▏"
	if tick%2 == 1 {
		caret = " "
	}
	cw := lipgloss.Width(caret)
	textW := maxBuf - cw
	if textW < 1 {
		textW = 1
	}
	return prefix + truncate(buf, textW) + caret
}

func lineFormModalBody(innerW, field, tick int, nameBuf, cmdBuf, footer string) []string {
	nameMark := "  "
	cmdMark := "  "
	if field == lineFormNameField {
		nameMark = "> "
	} else {
		cmdMark = "> "
	}
	nPrefix := nameMark + "Name: "
	cPrefix := cmdMark + "$ "
	nameLine := prefixedInputLine(nPrefix, nameBuf, innerW, field == lineFormNameField, tick)
	cmdLine := prefixedInputLine(cPrefix, cmdBuf, innerW, field == lineFormCmdField, tick)
	return []string{
		"Tab switches name / command.",
		"",
		nameLine,
		cmdLine,
		"",
		footer,
	}
}

func (m *model) dockVisibleCount() int {
	n := len(m.ids)
	if n == 0 {
		return 0
	}
	avail := m.height - 1 // footer row
	if avail < 2 {
		return min(9, n)
	}
	preferredLog := 3
	cap := avail - preferredLog
	if cap < 1 {
		cap = 1
	}
	return min(9, n, cap)
}

func (m *model) clampDockScroll(visible int) {
	if visible <= 0 || len(m.ids) == 0 {
		m.dockScroll = 0
		return
	}
	if len(m.ids) <= visible {
		m.dockScroll = 0
		return
	}
	maxScroll := len(m.ids) - visible
	if m.dockScroll < 0 {
		m.dockScroll = 0
	}
	if m.dockScroll > maxScroll {
		m.dockScroll = maxScroll
	}
	if m.selected < m.dockScroll {
		m.dockScroll = m.selected
	}
	if m.selected >= m.dockScroll+visible {
		m.dockScroll = m.selected - visible + 1
	}
}

// layoutHeights returns log row count and dock row count (footer is separate).
func (m *model) layoutHeights() (logH, dockRows int) {
	avail := m.height - 1
	dockRows = m.dockVisibleCount()
	m.clampDockScroll(dockRows)
	logH = avail - dockRows
	if logH < 1 {
		logH = 1
		if dockRows > avail-logH {
			dockRows = max(0, avail-logH)
			m.clampDockScroll(dockRows)
		}
	}
	return logH, dockRows
}

func (m *model) renderDock(dockRows int) string {
	if dockRows <= 0 || len(m.ids) == 0 {
		return ""
	}
	w := m.width
	if w < 1 {
		w = 1
	}
	selStyle := lipgloss.NewStyle().Background(lipgloss.Color("235")).Foreground(lipgloss.Color("252")).Width(w)
	lines := make([]string, 0, dockRows)
	for r := 0; r < dockRows; r++ {
		idx := m.dockScroll + r
		if idx >= len(m.ids) {
			break
		}
		id := m.ids[idx]
		st := "(?)"
		if s, ok := m.store.Get(id); ok {
			st = string(s)
		}
		hot := "· "
		if idx < 9 {
			hot = fmt.Sprintf("%d ", idx+1)
		}
		cmd := ""
		if idx < len(m.dockCmd) {
			cmd = m.dockCmd[idx]
		}
		name := ""
		if idx < len(m.processes) {
			name = m.processes[idx]
		}
		if name == "" {
			name = string(id)
		}
		suffix := fmt.Sprintf(" [%s]", st)
		bar := "  "
		if idx == m.selected {
			bar = "▌ "
		}
		prefix := hot + bar
		sep := " · "
		sepW := lipgloss.Width(sep)
		midBudget := w - lipgloss.Width(prefix) - lipgloss.Width(suffix)
		if midBudget < sepW+8 {
			midBudget = sepW + 8
		}
		inner := midBudget - sepW
		if inner < 6 {
			inner = 6
		}
		nameBudget := min(28, inner/2)
		nameShown := truncate(name, nameBudget)
		cmdBudget := inner - lipgloss.Width(nameShown)
		if cmdBudget < 6 {
			cmdBudget = 6
			nameShown = truncate(name, max(1, inner-cmdBudget))
		}
		cmdShown := truncate(cmd, cmdBudget)
		raw := prefix + nameShown + sep + cmdShown + suffix
		raw = padRight(truncate(raw, w), w)
		line := raw
		if idx == m.selected {
			line = selStyle.Render(raw)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
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
	dock := make([]string, 0, len(demos))
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
		dock = append(dock, formatExecLine(shCmd, args))
	}

	return &model{
		sup:       sup,
		store:     store,
		bus:       bus,
		sub:       bus.Subscribe(512),
		processes: names,
		dockCmd:   dock,
		ids:       ids,
		selected:  0,
		events:    []string{"[o] (imux) merged log — n new (name+shell, Tab) · e edit · dock ↑↓ 1–9 · Enter inspector · s/t/k/z/v/y."},
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

func (m *model) refreshInspector() {
	id := m.currentID()
	name := m.currentName()
	stStr := "(unknown)"
	if st, ok := m.store.Get(id); ok {
		stStr = string(st)
	}
	header := []string{
		fmt.Sprintf("Process: %s (%s)", name, id),
		fmt.Sprintf("State: %s", stStr),
		"",
	}
	if m.sup == nil {
		m.inspectLines = append(header, "No supervisor.")
		return
	}
	pid, live := m.sup.CurrentPID(id)
	if m.inspectFocusID != id {
		m.inspectCPU = nil
		m.inspectFocusID = id
	}
	if !live {
		m.inspectLines = append(header, "OS process not running (no pid).")
		return
	}
	detail, next, notes := inspect.Build(pid, m.inspectCPU)
	m.inspectCPU = next
	m.inspectLines = append(header, detail...)
	if len(notes) > 0 {
		m.inspectLines = append(m.inspectLines, "")
		for _, n := range notes {
			m.inspectLines = append(m.inspectLines, "— "+n)
		}
	}
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

func (m *model) tryAddProcess() {
	line := strings.TrimSpace(m.addBuf)
	if line == "" {
		m.appendLogLine("[error] add process: empty command")
		return
	}
	if m.sup == nil {
		m.appendLogLine("[error] add process: no supervisor")
		return
	}
	sh, shellArgs := shellWrapUserCommand(line)
	ctx := context.Background()
	name := sanitizeDisplayName(m.addNameBuf)
	if name == "" {
		name = nameFromCommandLine(line)
	}
	if name == "" {
		name = "proc"
	}
	for tries := 0; tries < 64; tries++ {
		m.nextUserSeq++
		id := core.ProcessID(fmt.Sprintf("u%d", m.nextUserSeq))
		spec := core.ProcessSpec{
			ID:      id,
			Name:    name,
			Command: sh,
			Args:    shellArgs,
			Restart: core.RestartConfig{Policy: core.RestartNever},
		}
		err := m.sup.Register(ctx, spec)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				continue
			}
			m.appendLogLine(fmt.Sprintf("[error] add process: register: %v", err))
			return
		}
		if err := m.sup.Start(ctx, id); err != nil {
			m.appendLogLine(fmt.Sprintf("[error] add process %s: start failed (registered, use s to retry): %v", id, err))
		} else {
			m.appendLogLine(fmt.Sprintf("[ok] added process %s (%s)", id, name))
		}
		m.refreshProcs()
		for i, pid := range m.ids {
			if pid == id {
				m.selected = i
				break
			}
		}
		m.resetLineOverlay()
		return
	}
	m.appendLogLine("[error] add process: could not allocate id")
}

func (m *model) resetLineOverlay() {
	m.addBuf = ""
	m.addNameBuf = ""
	m.editBuf = ""
	m.editNameBuf = ""
	m.editTargetID = ""
	m.lineOverlayField = lineFormNameField
	m.overlay = overlayNone
}

func (m *model) tryEditProcess() {
	line := strings.TrimSpace(m.editBuf)
	id := m.editTargetID
	if id == "" {
		m.resetLineOverlay()
		return
	}
	if line == "" {
		m.appendLogLine("[error] edit process: empty command")
		return
	}
	if m.sup == nil {
		m.appendLogLine("[error] edit process: no supervisor")
		return
	}
	st, ok := m.store.Get(id)
	if ok && (st == core.StateRunning || st == core.StateStarting || st == core.StatePaused || st == core.StateStopping) {
		m.appendLogLine("[error] edit process: stop the process before editing the command")
		return
	}
	ctx := context.Background()
	specs, err := m.sup.List(ctx)
	if err != nil {
		m.appendLogLine(fmt.Sprintf("[error] edit process: list: %v", err))
		return
	}
	var oldSpec core.ProcessSpec
	var found bool
	for _, sp := range specs {
		if sp.ID == id {
			oldSpec = sp
			found = true
			break
		}
	}
	if !found {
		m.appendLogLine("[error] edit process: process not found")
		return
	}
	if err := m.sup.Unregister(ctx, id); err != nil {
		m.appendLogLine(fmt.Sprintf("[error] edit process: unregister: %v", err))
		return
	}
	sh, shellArgs := shellWrapUserCommand(line)
	name := sanitizeDisplayName(m.editNameBuf)
	if name == "" {
		name = nameFromCommandLine(line)
	}
	if name == "" {
		name = "proc"
	}
	spec := core.ProcessSpec{
		ID:      id,
		Name:    name,
		Command: sh,
		Args:    shellArgs,
		Restart: core.RestartConfig{Policy: core.RestartNever},
	}
	if err := m.sup.Register(ctx, spec); err != nil {
		m.appendLogLine(fmt.Sprintf("[error] edit process: register: %v", err))
		if rerr := m.sup.Register(ctx, oldSpec); rerr != nil {
			m.appendLogLine(fmt.Sprintf("[error] edit process: could not restore previous definition: %v", rerr))
		} else {
			m.appendLogLine("[ok] edit aborted; restored previous command")
		}
		m.refreshProcs()
		m.resetLineOverlay()
		return
	}
	m.appendLogLine(fmt.Sprintf("[ok] updated definition for %s (%s); press s when you want it running", id, name))
	m.refreshProcs()
	for i, pid := range m.ids {
		if pid == id {
			m.selected = i
			break
		}
	}
	m.resetLineOverlay()
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
	dock := make([]string, len(specs))
	for i, sp := range specs {
		names[i] = sp.Name
		ids[i] = sp.ID
		dock[i] = formatExecLine(sp.Command, sp.Args)
	}
	m.processes = names
	m.dockCmd = dock
	m.ids = ids
	if m.selected >= len(m.ids) {
		m.selected = len(m.ids) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.clampDockScroll(m.dockVisibleCount())
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
		if m.overlay == overlayInspector && m.tick%3 == 0 {
			m.refreshInspector()
		}
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampDockScroll(m.dockVisibleCount())
	case tea.KeyMsg:
		if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
			edit := m.overlay == overlayEditProcess
			if msg.String() == "?" {
				m.helpReturnTo = m.overlay
				m.overlay = overlayHelp
				return m, nil
			}
			switch msg.String() {
			case "ctrl+c":
				m.shutdownProcs()
				return m, tea.Quit
			case "esc":
				m.resetLineOverlay()
			case "enter":
				if edit {
					m.tryEditProcess()
				} else {
					m.tryAddProcess()
				}
			case "backspace":
				if edit {
					if m.lineOverlayField == lineFormNameField {
						m.editNameBuf = trimLastRune(m.editNameBuf)
					} else {
						m.editBuf = trimLastRune(m.editBuf)
					}
				} else {
					if m.lineOverlayField == lineFormNameField {
						m.addNameBuf = trimLastRune(m.addNameBuf)
					} else {
						m.addBuf = trimLastRune(m.addBuf)
					}
				}
			case "tab":
				if m.lineOverlayField == lineFormNameField {
					m.lineOverlayField = lineFormCmdField
				} else {
					m.lineOverlayField = lineFormNameField
				}
			default:
				switch msg.Type {
				case tea.KeyRunes:
					if edit {
						if m.lineOverlayField == lineFormNameField {
							m.editNameBuf += string(msg.Runes)
							if len(m.editNameBuf) > 256 {
								m.editNameBuf = m.editNameBuf[:256]
							}
						} else {
							m.editBuf += string(msg.Runes)
							if len(m.editBuf) > 4000 {
								m.editBuf = m.editBuf[:4000]
							}
						}
					} else {
						if m.lineOverlayField == lineFormNameField {
							m.addNameBuf += string(msg.Runes)
							if len(m.addNameBuf) > 256 {
								m.addNameBuf = m.addNameBuf[:256]
							}
						} else {
							m.addBuf += string(msg.Runes)
							if len(m.addBuf) > 4000 {
								m.addBuf = m.addBuf[:4000]
							}
						}
					}
				case tea.KeySpace:
					if edit {
						if m.lineOverlayField == lineFormNameField {
							m.editNameBuf += " "
						} else {
							m.editBuf += " "
						}
					} else {
						if m.lineOverlayField == lineFormNameField {
							m.addNameBuf += " "
						} else {
							m.addBuf += " "
						}
					}
				}
			}
			return m, nil
		}
		switch msg.String() {
		case "ctrl+c", "q":
			m.shutdownProcs()
			return m, tea.Quit
		case "1", "2", "3", "4", "5", "6", "7", "8", "9":
			if m.overlay == overlayHelp {
				break
			}
			idx := int(msg.String()[0] - '1')
			if idx < len(m.ids) {
				m.selected = idx
				m.clampDockScroll(m.dockVisibleCount())
			}
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
		case "n":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
				break
			}
			m.overlay = overlayAddProcess
			m.addBuf = ""
			m.addNameBuf = ""
			m.lineOverlayField = lineFormNameField
		case "e":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
				break
			}
			id := m.currentID()
			if id == "" {
				break
			}
			st, ok := m.store.Get(id)
			if ok && (st == core.StateRunning || st == core.StateStarting || st == core.StatePaused || st == core.StateStopping) {
				m.appendLogLine("[error] edit: stop the process first")
				break
			}
			ctx := context.Background()
			specs, err := m.sup.List(ctx)
			if err != nil {
				m.appendLogLine(fmt.Sprintf("[error] edit: %v", err))
				break
			}
			var spec *core.ProcessSpec
			for i := range specs {
				if specs[i].ID == id {
					spec = &specs[i]
					break
				}
			}
			if spec == nil {
				m.appendLogLine("[error] edit: process not found")
				break
			}
			m.editTargetID = id
			m.editBuf = innerCommandForEdit(*spec)
			m.editNameBuf = spec.Name
			m.lineOverlayField = lineFormNameField
			m.overlay = overlayEditProcess
		case "enter":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
				break
			}
			if m.currentID() != "" {
				m.overlay = overlayInspector
				m.refreshInspector()
			}
		case "i":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			} else {
				m.overlay = overlayInspector
				m.refreshInspector()
			}
		case "r":
			if m.overlay == overlayInspector {
				m.refreshInspector()
			}
		case "up":
			if m.overlay == overlayHelp {
				break
			}
			if m.selected > 0 {
				m.selected--
				m.clampDockScroll(m.dockVisibleCount())
			}
		case "k":
			if m.overlay == overlayHelp {
				break
			}
			if m.sup != nil {
				ctx := context.Background()
				if id := m.currentID(); id != "" {
					m.appendCtlErr("kill", m.sup.Kill(ctx, id))
				}
			}
		case "down":
			if m.overlay == overlayHelp {
				break
			}
			if m.selected < len(m.processes)-1 {
				m.selected++
				m.clampDockScroll(m.dockVisibleCount())
			}
		case ",", "<":
			if m.overlay == overlayHelp {
				break
			}
			if m.selected > 0 {
				m.selected--
				m.clampDockScroll(m.dockVisibleCount())
			}
		case ".", ">":
			if m.overlay == overlayHelp {
				break
			}
			if m.selected < len(m.processes)-1 {
				m.selected++
				m.clampDockScroll(m.dockVisibleCount())
			}
		default:
			if m.overlay == overlayHelp {
				break
			}
			if m.sup != nil {
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

	logH, dockRows := m.layoutHeights()
	logBlock := m.renderBody(logH)
	if dockRows > 0 {
		logBlock = lipgloss.JoinVertical(lipgloss.Left, logBlock, m.renderDock(dockRows))
	}

	bodyH := m.height - 1
	if bodyH < 1 {
		bodyH = 1
	}
	if m.overlay != overlayNone {
		modal := m.renderModal()
		logBlock = compositeLogWithModal(logBlock, modal, m.width, bodyH)
	}

	footer := m.renderFooter()
	footerStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(footer)
	return lipgloss.JoinVertical(lipgloss.Left, logBlock, footerStyled)
}

func (m *model) renderFooter() string {
	var s string
	switch m.overlay {
	case overlayHelp:
		if m.helpReturnTo == overlayNone {
			s = "Esc or ? closes help"
		} else {
			s = "Esc or ? returns"
		}
	case overlayInspector:
		s = "Esc or Enter closes · r refresh"
	case overlayAddProcess:
		s = "Esc cancels · Enter adds"
	case overlayEditProcess:
		s = "Esc cancels · Enter saves · s starts"
	default:
		s = "? help · q quit"
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
	if m.overlay == overlayInspector {
		maxW = min(72, m.width-4)
		maxH = min(22, m.height-4)
	}
	if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
		maxW = min(72, m.width-4)
		maxH = min(16, m.height-4)
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
			"  ↑ ↓           move selection in the bottom dock",
			"  1-9           jump to process slot (first nine)",
			"  s t k z v y   start / stop / kill / pause / continue / restart",
			"  , or .        previous / next process (same as arrows)",
			"  Enter or i    inspector (Esc or Enter closes, r refreshes)",
			"  n             new process (name + command, Tab switches field)",
			"  e             edit name + command (stop first; Enter saves, then s to run)",
			"  ? Esc         help · close overlay",
			"  q Ctrl+c      quit (stops running demos)",
			"",
			fmt.Sprintf("Focus: %s (%s)", proc, m.currentID()),
		}
	case overlayInspector:
		title = "Inspector"
		bodyLines = append(append([]string(nil), m.inspectLines...), "", "Esc or Enter closes · r refresh · ? help")
	case overlayAddProcess:
		title = "New process"
		bodyLines = append([]string{"Wrapped like imux run (sh -c or cmd /C)."},
			lineFormModalBody(innerW, m.lineOverlayField, m.tick, m.addNameBuf, m.addBuf, "Esc cancel · Enter register+start")...)
	case overlayEditProcess:
		title = "Edit process"
		bodyLines = append([]string{fmt.Sprintf("id %s — same slot.", m.editTargetID)},
			lineFormModalBody(innerW, m.lineOverlayField, m.tick, m.editNameBuf, m.editBuf, "Esc cancel · Enter save (pending — s to start)")...)
	default:
		title = ""
		bodyLines = nil
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
