package tui

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"imux/internal/core"
	"imux/internal/inspect"
	"imux/internal/sessionlog"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	xterm "github.com/charmbracelet/x/term"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayInspector
	overlayHelp
	overlayAddProcess
	overlayEditProcess
	overlayLogFilter
	overlayKillSignal
)

// dockSlot is one logical process row. Tombstones (Deleted) stay in stable session order
// so merged-log colors stay aligned; they are omitted from the visible dock.
type dockSlot struct {
	ID      core.ProcessID
	Name    string
	DockCmd string
	Deleted bool
}

type model struct {
	width        int
	height       int
	overlay      overlayKind
	helpReturnTo overlayKind
	processes    []string         // visible dock only (derived from slots)
	dockCmd      []string         // visible dock only
	ids          []core.ProcessID // visible dock only
	slots        []dockSlot       // stable order incl. tombstones; drives log color keys
	selected     int
	dockScroll   int // first visible row index when len(ids) > dock capacity
	tick         int

	pendingQuit         bool
	pendingQuitDeadline time.Time // second q/Ctrl+c quits before this; zero when not armed

	pendingDelete         bool
	pendingDeleteDeadline time.Time // second d removes slot before this; zero when not armed
	pendingDeleteID       core.ProcessID

	pendingStop         bool
	pendingStopDeadline time.Time
	pendingStopID       core.ProcessID

	pendingStopAll         bool
	pendingStopAllDeadline time.Time

	killSignalTargetID core.ProcessID
	killSignalSel      int
	killSignalBulkAll  bool // true when overlay opened with K (all running slots)

	// Ephemeral UI messages (appendToast); shown in the footer, not in the merged log.
	toastText     string
	toastDeadline time.Time
	toastKind     ToastKind

	sup   *core.ExecSupervisor
	store core.StateStore
	bus   core.EventBus
	sub   <-chan core.Event

	slog          *sessionlog.SessionLog
	opts          Options
	showStdout    bool
	showStderr    bool
	logTimePrec   logTimePrecision
	logScroll     int
	logHScroll    int  // horizontal pan of log lines (terminal cells)
	logWordWrap   bool // when true, log lines wrap to the viewport width (no horizontal pan)
	filt          *compiledFilter
	filterPattern string
	filterInp     textinput.Model
	matchedIdx    []int64
	lastBuiltN    int64
	matchSig      string

	inspectLines   []string
	inspectCPU     *inspect.CPUSample
	inspectFocusID core.ProcessID

	nextUserSeq      int
	addNameInp       textinput.Model
	addCmdInp        textinput.Model
	editNameInp      textinput.Model
	editCmdInp       textinput.Model
	editTargetID     core.ProcessID
	lineOverlayField int    // lineFormNameField or lineFormCmdField
	modalErr         string // add/edit overlay: last save/register failure (footer toasts are hidden there)

	// lastExitCode is set from supervisor bus messages for dock display after exit/fail.
	lastExitCode map[core.ProcessID]int
}

type tickMsg time.Time

// busEventMsg carries one supervisor event into the Bubble Tea loop as soon as it is
// published, so output is not delayed until the next UI tick (~400ms).
type busEventMsg core.Event

const (
	tickInterval = 400 * time.Millisecond

	// pendingConfirmWindow is how long the user has to press the confirming key again
	// (quit, delete slot, stop, stop-all).
	pendingConfirmWindow = 3 * time.Second

	// defaultToastLifetime is how long footer status toasts stay visible.
	defaultToastLifetime = 3 * time.Second
)

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *model) listenCmd() tea.Cmd {
	if m.sub == nil {
		return nil
	}
	sub := m.sub
	return func() tea.Msg {
		return busEventMsg(<-sub)
	}
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

// effectiveDockLabel is how a slot reads in the dock (same idea as nameForID on slots).
func effectiveDockLabel(name string, id core.ProcessID) string {
	n := strings.TrimSpace(name)
	if n != "" {
		return n
	}
	return string(id)
}

// displayNameConflicts reports whether candidate matches another slot's effective dock label
// (case-insensitive). ignoreID skips that slot (use "" when registering a new process).
func displayNameConflicts(specs []core.ProcessSpec, ignoreID core.ProcessID, candidate string) (otherID core.ProcessID, yes bool) {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return "", false
	}
	for _, sp := range specs {
		if ignoreID != "" && sp.ID == ignoreID {
			continue
		}
		if strings.EqualFold(effectiveDockLabel(sp.Name, sp.ID), candidate) {
			return sp.ID, true
		}
	}
	return "", false
}

// resolvedNameFromBuffers is the display name tryAdd/tryEdit would use for these buffers.
func resolvedNameFromBuffers(nameBuf, cmdBuf string) string {
	name := sanitizeDisplayName(nameBuf)
	line := strings.TrimSpace(cmdBuf)
	if name == "" {
		name = nameFromCommandLine(line)
	}
	if name == "" {
		name = "proc"
	}
	return name
}

// nameEntryConflicts is true when the current add/edit draft name would collide with another slot's dock label.
func (m *model) nameEntryConflicts(edit bool) bool {
	if m.sup == nil {
		return false
	}
	var nameBuf, cmdBuf string
	var self core.ProcessID
	if edit {
		if m.editTargetID == "" {
			return false
		}
		nameBuf, cmdBuf, self = m.editNameInp.Value(), m.editCmdInp.Value(), m.editTargetID
	} else {
		nameBuf, cmdBuf = m.addNameInp.Value(), m.addCmdInp.Value()
	}
	name := resolvedNameFromBuffers(nameBuf, cmdBuf)
	ctx := context.Background()
	specs, err := m.sup.List(ctx)
	if err != nil {
		return false
	}
	_, dup := displayNameConflicts(specs, self, name)
	return dup
}

// modalSaveErrMessage turns supervisor/API errors into short copy for the add/edit dialog.
func modalSaveErrMessage(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	switch {
	case strings.Contains(s, "still has an active child"):
		return "Save still blocked after stopping the process — try Enter again, or Esc to discard."
	case strings.HasPrefix(s, "replace spec: "):
		return strings.TrimSpace(strings.TrimPrefix(s, "replace spec: "))
	default:
		return s
	}
}

func appendModalSaveErr(body []string, innerW int, msg string) []string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return body
	}
	// Full message; wrapModalLines will soft-wrap to innerW (truncate() was cell-wrong and hid the tail).
	return append(body, "", msg)
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
	dockBase := lipgloss.NewStyle().
		Background(lipgloss.Color("236")).
		Foreground(lipgloss.Color("247")).
		Width(w)
	dockSel := lipgloss.NewStyle().
		Background(lipgloss.Color("238")).
		Foreground(lipgloss.Color("252")).
		Width(w)
	lines := make([]string, 0, dockRows)
	for r := 0; r < dockRows; r++ {
		idx := m.dockScroll + r
		if idx >= len(m.ids) {
			break
		}
		id := m.ids[idx]
		st := "(?)"
		if s, ok := m.store.Get(id); ok {
			st = dockStatusWithExit(m, id, s)
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
		line := dockBase.Render(raw)
		if idx == m.selected {
			line = dockSel.Render(raw)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func newModel(opts Options) (*model, error) {
	slog, err := sessionlog.Open(opts.TeePath)
	if err != nil {
		return nil, err
	}
	pat, perr := ParseLogFilter(opts.LogFilter)
	if perr != nil {
		_ = slog.Close()
		return nil, perr
	}
	cf, cerr := compileFilter(pat)
	if cerr != nil {
		_ = slog.Close()
		return nil, cerr
	}

	bus := core.NewChanEventBus()
	store := core.NewMapStateStore()
	sup := core.NewExecSupervisor(bus, store)
	sup.SetStopGrace(10 * time.Second)

	m := &model{
		sup:           sup,
		store:         store,
		bus:           bus,
		sub:           bus.Subscribe(512),
		processes:     nil,
		dockCmd:       nil,
		ids:           nil,
		selected:      0,
		slog:          slog,
		opts:          opts,
		showStdout:    true,
		showStderr:    true,
		filt:          cf,
		filterPattern: pat,
		lastBuiltN:    -1,
	}
	m.addNameInp = newNameLineTI()
	m.addCmdInp = newCmdLineTI()
	m.editNameInp = newNameLineTI()
	m.editCmdInp = newCmdLineTI()
	m.filterInp = newFilterPatternTI()
	m.filterInp.SetValue(pat)
	if len(opts.Bootstrap) > 0 {
		if err := m.applyBootstrap(opts.Bootstrap); err != nil {
			_ = slog.Close()
			return nil, err
		}
	}
	return m, nil
}

func (m *model) applyBootstrap(procs []BootstrapProc) error {
	if m.sup == nil {
		return fmt.Errorf("bootstrap: no supervisor")
	}
	ctx := context.Background()
	for _, p := range procs {
		line := strings.TrimSpace(p.Line)
		if line == "" {
			return fmt.Errorf("bootstrap: empty command for %q", p.ID)
		}
		sh, shellArgs := shellWrapUserCommand(line)
		id := core.ProcessID(strings.TrimSpace(p.ID))
		spec := core.ProcessSpec{
			ID:      id,
			Name:    string(id),
			Command: sh,
			Args:    shellArgs,
			Restart: core.RestartConfig{Policy: core.RestartNever},
		}
		if err := m.sup.Register(ctx, spec); err != nil {
			return fmt.Errorf("bootstrap register %s: %w", id, err)
		}
		if err := m.sup.Start(ctx, id); err != nil {
			return fmt.Errorf("bootstrap start %s: %w", id, err)
		}
	}
	m.refreshProcs()
	if len(m.ids) > 0 {
		m.selected = 0
	}
	return nil
}

func (m *model) dispose() {
	if m.slog != nil {
		_ = m.slog.Close()
		m.slog = nil
	}
}

func (m *model) logMatchSig() string {
	return fmt.Sprintf("%s|%v|%v", m.filterPattern, m.showStdout, m.showStderr)
}

func (m *model) forceLogRebuild() {
	m.lastBuiltN = -1
	m.matchedIdx = nil
	m.matchSig = ""
	m.logHScroll = 0
}

func (m *model) syncLogIndices() error {
	if m.slog == nil {
		return nil
	}
	n, err := m.slog.LineCount()
	if err != nil {
		return err
	}
	sig := m.logMatchSig()
	if m.lastBuiltN < 0 || n < m.lastBuiltN || sig != m.matchSig {
		idx, err := MatchLineIndices(m.slog, m.filt, m.showStdout, m.showStderr)
		if err != nil {
			return err
		}
		m.matchedIdx = idx
		m.lastBuiltN = n
		m.matchSig = sig
		return nil
	}
	if n > m.lastBuiltN {
		var batch []int64
		for i := n - 1; i >= m.lastBuiltN; i-- {
			rec, err := m.slog.ReadLine(i)
			if err != nil {
				return err
			}
			if !isChildStream(rec) {
				continue
			}
			if !passesStreamToggles(rec, m.showStdout, m.showStderr) {
				continue
			}
			if !m.filt.match(flatRecord(rec)) {
				continue
			}
			batch = append(batch, i)
		}
		m.matchedIdx = append(batch, m.matchedIdx...)
		// matchedIdx is newest-first; new matches are prepended. If the user has
		// scrolled up (logScroll > 0), bump scroll by the prepend size so the same
		// lines stay on screen instead of sliding toward the tail.
		if m.logScroll > 0 && len(batch) > 0 {
			m.logScroll += len(batch)
		}
		m.lastBuiltN = n
	}
	return nil
}

func (m *model) applyLogFilter() {
	m.overlay = overlayNone
	m.filterPattern = strings.TrimSpace(m.filterInp.Value())
	cf, err := compileFilter(m.filterPattern)
	if err != nil {
		return
	}
	m.filt = cf
	m.forceLogRebuild()
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

func (m *model) appendToast(kind ToastKind, msg string) {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return
	}
	m.toastText = msg
	m.toastKind = kind
	m.toastDeadline = time.Now().Add(defaultToastLifetime)
}

func (m *model) nameForID(id core.ProcessID) string {
	for i := range m.slots {
		if m.slots[i].ID != id {
			continue
		}
		if n := strings.TrimSpace(m.slots[i].Name); n != "" {
			return n
		}
		break
	}
	return string(id)
}

func (m *model) appendCtlErrFor(op string, id core.ProcessID, displayName string, err error) {
	dn := strings.TrimSpace(displayName)
	idStr := string(id)
	if dn == "" {
		dn = idStr
	}
	subject := dn
	if dn != idStr && idStr != "" {
		subject = fmt.Sprintf("%s (%s)", dn, idStr)
	}
	if err == nil {
		m.appendToast(ToastOK, fmt.Sprintf("%s %s", op, subject))
		return
	}
	m.appendToast(ToastErr, fmt.Sprintf("%s %s: %v", op, subject, err))
}

func (m *model) appendCtlErr(op string, err error) {
	m.appendCtlErrFor(op, m.currentID(), m.currentName(), err)
}

func (m *model) startAllGracefulCmd() tea.Cmd {
	if m.sup == nil {
		return nil
	}
	var cmds []tea.Cmd
	var names []string
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok {
			continue
		}
		switch st {
		case core.StatePending, core.StateExited, core.StateFailed:
			id := id
			name := m.nameForID(id)
			sup := m.sup
			names = append(names, name)
			cmds = append(cmds, func() tea.Msg {
				err := sup.Start(context.Background(), id)
				return supOpDoneMsg{op: "run", id: id, name: name, err: err, bulk: true}
			})
		default:
			// running, starting, paused, stopping: skip
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	m.appendToast(ToastOK, fmt.Sprintf("Run all (%d): %s", len(names), strings.Join(names, ", ")))
	return tea.Batch(cmds...)
}

func (m *model) stopAllGracefulCmd() tea.Cmd {
	if m.sup == nil {
		return nil
	}
	var cmds []tea.Cmd
	var names []string
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok {
			continue
		}
		switch st {
		case core.StateRunning, core.StateStarting, core.StatePaused:
			id := id
			name := m.nameForID(id)
			sup := m.sup
			names = append(names, name)
			cmds = append(cmds, func() tea.Msg {
				err := sup.Stop(context.Background(), id)
				return supOpDoneMsg{op: "terminate", id: id, name: name, err: err, bulk: true}
			})
		default:
			// pending, exited, failed, stopping: skip
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	m.appendToast(ToastOK, fmt.Sprintf("Terminate all (%d): %s", len(names), strings.Join(names, ", ")))
	return tea.Batch(cmds...)
}

func (m *model) pauseAllGracefulCmd() tea.Cmd {
	if m.sup == nil {
		return nil
	}
	var cmds []tea.Cmd
	var names []string
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok || st != core.StateRunning {
			continue
		}
		id := id
		name := m.nameForID(id)
		sup := m.sup
		names = append(names, name)
		cmds = append(cmds, func() tea.Msg {
			err := sup.Pause(context.Background(), id)
			return supOpDoneMsg{op: "pause", id: id, name: name, err: err, bulk: true}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	m.appendToast(ToastOK, fmt.Sprintf("Pause all (%d): %s", len(names), strings.Join(names, ", ")))
	return tea.Batch(cmds...)
}

func (m *model) continueAllGracefulCmd() tea.Cmd {
	if m.sup == nil {
		return nil
	}
	var cmds []tea.Cmd
	var names []string
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok || st != core.StatePaused {
			continue
		}
		id := id
		name := m.nameForID(id)
		sup := m.sup
		names = append(names, name)
		cmds = append(cmds, func() tea.Msg {
			err := sup.Continue(context.Background(), id)
			return supOpDoneMsg{op: "continue", id: id, name: name, err: err, bulk: true}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	m.appendToast(ToastOK, fmt.Sprintf("Continue all (%d): %s", len(names), strings.Join(names, ", ")))
	return tea.Batch(cmds...)
}

func (m *model) restartAllGracefulCmd() tea.Cmd {
	if m.sup == nil {
		return nil
	}
	var cmds []tea.Cmd
	var names []string
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok || st == core.StateStopping {
			continue
		}
		id := id
		name := m.nameForID(id)
		sup := m.sup
		names = append(names, name)
		cmds = append(cmds, func() tea.Msg {
			err := sup.Restart(context.Background(), id)
			return supOpDoneMsg{op: "restart", id: id, name: name, err: err, bulk: true}
		})
	}
	if len(cmds) == 0 {
		return nil
	}
	m.appendToast(ToastOK, fmt.Sprintf("Restart all (%d): %s", len(names), strings.Join(names, ", ")))
	return tea.Batch(cmds...)
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

func exitCodeFromBusMessage(msg string) (int, bool) {
	const prefix = "exited with code "
	if !strings.HasPrefix(msg, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(msg[len(prefix):]))
	if err != nil {
		return 0, false
	}
	return n, true
}

func dockStatusWithExit(m *model, id core.ProcessID, st core.ProcessState) string {
	s := string(st)
	if st != core.StateExited && st != core.StateFailed {
		return s
	}
	if m.lastExitCode == nil {
		return s
	}
	code, ok := m.lastExitCode[id]
	if !ok {
		return s
	}
	return fmt.Sprintf("%s %d", s, code)
}

func (m *model) consumeBusEvent(e core.Event) {
	switch e.Type {
	case core.EventProcessOutput:
		if m.slog == nil {
			return
		}
		k := sessionlog.KindStdout
		msg := e.Message
		switch e.Stream {
		case "e":
			k = sessionlog.KindStderr
		case "o", "":
			k = sessionlog.KindStdout
		default:
			k = sessionlog.KindStderr
			msg = fmt.Sprintf("[stream %q] %s", e.Stream, e.Message)
		}
		_ = m.slog.Append(sessionlog.Record{
			T:    e.Timestamp,
			K:    k,
			ID:   string(e.ProcessID),
			Name: e.ProcessName,
			Msg:  msg,
		})
	case core.EventProcessExited, core.EventProcessFailed:
		if code, ok := exitCodeFromBusMessage(e.Message); ok {
			if m.lastExitCode == nil {
				m.lastExitCode = make(map[core.ProcessID]int)
			}
			m.lastExitCode[e.ProcessID] = code
		}
	case core.EventProcessStarting, core.EventProcessRunning:
		if m.lastExitCode != nil {
			delete(m.lastExitCode, e.ProcessID)
		}
	case core.EventProcessSignalSent:
		if m.slog == nil {
			return
		}
		_ = m.slog.Append(sessionlog.Record{
			T:    e.Timestamp,
			K:    sessionlog.KindMeta,
			ID:   string(e.ProcessID),
			Name: m.nameForID(e.ProcessID),
			Msg:  e.Message,
		})
	default:
		return
	}
}

func (m *model) drainEvents() {
	if m.sub == nil {
		return
	}
	for {
		select {
		case e := <-m.sub:
			m.consumeBusEvent(e)
		default:
			return
		}
	}
}

func (m *model) tryAddProcess() {
	m.modalErr = ""
	line := strings.TrimSpace(m.addCmdInp.Value())
	if line == "" {
		m.modalErr = "Add: empty command"
		return
	}
	if m.sup == nil {
		m.modalErr = "Add: no supervisor"
		return
	}
	sh, shellArgs := shellWrapUserCommand(line)
	ctx := context.Background()
	name := sanitizeDisplayName(m.addNameInp.Value())
	if name == "" {
		name = nameFromCommandLine(line)
	}
	if name == "" {
		name = "proc"
	}
	specs, err := m.sup.List(ctx)
	if err != nil {
		m.modalErr = fmt.Sprintf("Add: %v", err)
		return
	}
	if _, dup := displayNameConflicts(specs, "", name); dup {
		return
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
			m.modalErr = fmt.Sprintf("Register: %v", err)
			return
		}
		m.appendToast(ToastOK, fmt.Sprintf("Registered %s (%s); press r to run", id, name))
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
	m.modalErr = "Add: could not allocate id"
}

func (m *model) resetLineOverlay() {
	m.addNameInp.Reset()
	m.addCmdInp.Reset()
	m.editNameInp.Reset()
	m.editCmdInp.Reset()
	m.editTargetID = ""
	m.lineOverlayField = lineFormNameField
	m.modalErr = ""
	m.overlay = overlayNone
}

func (m *model) deletePrevalidate(id core.ProcessID) error {
	if m.sup == nil {
		return fmt.Errorf("no supervisor")
	}
	if id == "" {
		return fmt.Errorf("no process selected")
	}
	st, ok := m.store.Get(id)
	if !ok {
		return fmt.Errorf("unknown process")
	}
	if st == core.StateRunning || st == core.StateStarting || st == core.StatePaused || st == core.StateStopping {
		return fmt.Errorf("terminate the process before removing its slot")
	}
	return nil
}

func (m *model) tryDeleteProcess() {
	if m.sup == nil {
		return
	}
	id := m.currentID()
	if err := m.deletePrevalidate(id); err != nil {
		m.appendToast(ToastErr, "Delete: "+err.Error())
		return
	}
	ctx := context.Background()
	if err := m.sup.Unregister(ctx, id); err != nil {
		m.appendToast(ToastErr, fmt.Sprintf("Delete: %v", err))
		return
	}
	m.appendToast(ToastOK, fmt.Sprintf("Removed slot %s", id))
	if m.pendingStop && m.pendingStopID == id {
		m.clearPendingStop()
	}
	m.refreshProcs()
	if m.selected >= len(m.ids) {
		m.selected = len(m.ids) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
	m.clampDockScroll(m.dockVisibleCount())
	m.forceLogRebuild()
}

// openEditProcessFromMain opens the edit overlay for the selected dock slot.
// Saving while a child is running stops it automatically, then applies the new spec.
func (m *model) openEditProcessFromMain() tea.Cmd {
	if m.sup == nil {
		return nil
	}
	if m.overlay == overlayInspector {
		m.overlay = overlayNone
	}
	if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
		return nil
	}
	id := m.currentID()
	if id == "" {
		return nil
	}
	ctx := context.Background()
	specs, err := m.sup.List(ctx)
	if err != nil {
		m.appendToast(ToastErr, fmt.Sprintf("Edit: %v", err))
		return nil
	}
	var spec *core.ProcessSpec
	for i := range specs {
		if specs[i].ID == id {
			spec = &specs[i]
			break
		}
	}
	if spec == nil {
		m.appendToast(ToastErr, "Edit: process not found")
		return nil
	}
	m.editTargetID = id
	m.editNameInp.SetValue(spec.Name)
	m.editCmdInp.SetValue(innerCommandForEdit(*spec))
	m.lineOverlayField = lineFormNameField
	m.modalErr = ""
	m.overlay = overlayEditProcess
	m.syncLineFormWidths(m.lineFormInnerW())
	return m.refocusLineFormCurrent()
}

func (m *model) tryEditProcess() tea.Cmd {
	m.modalErr = ""
	line := strings.TrimSpace(m.editCmdInp.Value())
	id := m.editTargetID
	if id == "" {
		m.resetLineOverlay()
		return nil
	}
	if line == "" {
		m.modalErr = "Edit: empty command"
		return nil
	}
	if m.sup == nil {
		m.modalErr = "Edit: no supervisor"
		return nil
	}
	ctx := context.Background()
	specs, err := m.sup.List(ctx)
	if err != nil {
		m.modalErr = fmt.Sprintf("Edit: list: %v", err)
		return nil
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
		m.modalErr = "Edit: process not found"
		return nil
	}
	sh, shellArgs := shellWrapUserCommand(line)
	name := sanitizeDisplayName(m.editNameInp.Value())
	if name == "" {
		name = nameFromCommandLine(line)
	}
	if name == "" {
		name = "proc"
	}
	if _, dup := displayNameConflicts(specs, id, name); dup {
		return nil
	}
	spec := core.ProcessSpec{
		ID:      id,
		Name:    name,
		Command: sh,
		Args:    shellArgs,
		Env:     oldSpec.Env,
		Dir:     oldSpec.Dir,
		Restart: oldSpec.Restart,
	}
	err = m.sup.ReplaceSpec(ctx, id, spec)
	if err != nil && strings.Contains(err.Error(), "still has an active child") {
		return supStopThenReplaceSpecCmd(m.sup, id, name, spec)
	}
	if err != nil {
		m.modalErr = modalSaveErrMessage(err)
		m.refreshProcs()
		return nil
	}
	m.appendToast(ToastOK, fmt.Sprintf("Updated %s (%s); press r to run when ready", id, name))
	m.refreshProcs()
	for i, pid := range m.ids {
		if pid == id {
			m.selected = i
			break
		}
	}
	m.resetLineOverlay()
	return nil
}

func (m *model) handleReplaceSaveDone(msg supReplaceSaveDoneMsg) (tea.Model, tea.Cmd) {
	id := msg.id
	name := msg.name
	if msg.err != nil {
		m.modalErr = fmt.Sprintf("Edit: replace blocked while running; terminate failed: %v", msg.err)
		m.refreshProcs()
		return m, m.refocusLineFormCurrent()
	}
	if msg.replaceErr != nil {
		m.modalErr = modalSaveErrMessage(msg.replaceErr)
		m.refreshProcs()
		return m, m.refocusLineFormCurrent()
	}
	m.appendToast(ToastOK, fmt.Sprintf("Terminated and saved %s (%s); press r to run when ready", id, name))
	m.refreshProcs()
	for i, pid := range m.ids {
		if pid == id {
			m.selected = i
			break
		}
	}
	m.resetLineOverlay()
	return m, nil
}

func (m *model) refreshProcs() {
	if m.sup == nil {
		return
	}
	specs, err := m.sup.List(context.Background())
	if err != nil {
		return
	}

	specIDs := make(map[core.ProcessID]struct{}, len(specs))
	for _, sp := range specs {
		specIDs[sp.ID] = struct{}{}
	}

	for i := range m.slots {
		if m.slots[i].Deleted {
			continue
		}
		if _, ok := specIDs[m.slots[i].ID]; !ok {
			m.slots[i].Deleted = true
			if m.lastExitCode != nil {
				delete(m.lastExitCode, m.slots[i].ID)
			}
		}
	}

	for _, sp := range specs {
		found := false
		for i := range m.slots {
			if m.slots[i].ID != sp.ID {
				continue
			}
			found = true
			m.slots[i].Deleted = false
			m.slots[i].Name = sp.Name
			m.slots[i].DockCmd = formatExecLine(sp.Command, sp.Args)
			break
		}
		if !found {
			m.slots = append(m.slots, dockSlot{
				ID:      sp.ID,
				Name:    sp.Name,
				DockCmd: formatExecLine(sp.Command, sp.Args),
				Deleted: false,
			})
		}
	}

	m.rebuildVisibleDockCaches()
}

func (m *model) rebuildVisibleDockCaches() {
	names := make([]string, 0, len(m.slots))
	ids := make([]core.ProcessID, 0, len(m.slots))
	cmds := make([]string, 0, len(m.slots))
	for _, sl := range m.slots {
		if sl.Deleted {
			continue
		}
		names = append(names, sl.Name)
		ids = append(ids, sl.ID)
		cmds = append(cmds, sl.DockCmd)
	}
	m.processes = names
	m.ids = ids
	m.dockCmd = cmds
	if len(m.ids) == 0 {
		m.selected = 0
	} else {
		if m.selected >= len(m.ids) {
			m.selected = len(m.ids) - 1
		}
		if m.selected < 0 {
			m.selected = 0
		}
	}
	m.clampDockScroll(m.dockVisibleCount())
}

func (m *model) clearPendingQuit() {
	m.pendingQuit = false
	m.pendingQuitDeadline = time.Time{}
}

func (m *model) clearPendingDelete() {
	m.pendingDelete = false
	m.pendingDeleteDeadline = time.Time{}
	m.pendingDeleteID = ""
}

func (m *model) clearPendingStop() {
	m.pendingStop = false
	m.pendingStopDeadline = time.Time{}
	m.pendingStopID = ""
}

func (m *model) clearPendingStopAll() {
	m.pendingStopAll = false
	m.pendingStopAllDeadline = time.Time{}
}

func (m *model) closeKillSignalOverlay() {
	m.killSignalTargetID = ""
	m.killSignalSel = 0
	m.killSignalBulkAll = false
	if m.overlay == overlayKillSignal {
		m.overlay = overlayNone
	}
}

func (m *model) killableRunningCount() int {
	n := 0
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok {
			continue
		}
		switch st {
		case core.StateRunning, core.StateStarting, core.StatePaused:
			n++
		}
	}
	return n
}

func (m *model) applyKillSignalChoice() tea.Cmd {
	menu := killSignalMenu()
	if len(menu) == 0 || m.killSignalSel < 0 || m.killSignalSel >= len(menu) {
		m.closeKillSignalOverlay()
		return nil
	}
	if m.sup == nil {
		m.closeKillSignalOverlay()
		return nil
	}
	choice := menu[m.killSignalSel].sig
	if m.killSignalBulkAll {
		m.closeKillSignalOverlay()
		var cmds []tea.Cmd
		for _, id := range m.ids {
			st, ok := m.store.Get(id)
			if !ok {
				continue
			}
			switch st {
			case core.StateRunning, core.StateStarting, core.StatePaused:
				id := id
				name := m.nameForID(id)
				sup := m.sup
				ch := choice
				cmds = append(cmds, func() tea.Msg {
					err := sup.SendUserSignal(context.Background(), id, ch)
					return supOpDoneMsg{op: "signal", id: id, name: name, err: err}
				})
			}
		}
		if len(cmds) == 0 {
			return nil
		}
		return tea.Batch(cmds...)
	}
	id := m.killSignalTargetID
	if id == "" {
		m.closeKillSignalOverlay()
		return nil
	}
	m.closeKillSignalOverlay()
	return supSignalCmd(m.sup, id, m.nameForID(id), choice)
}

// stopArmEligible is true when Stop is meaningful for this process (matches bulk stop rules).
func (m *model) stopArmEligible(id core.ProcessID) bool {
	if id == "" {
		return false
	}
	st, ok := m.store.Get(id)
	if !ok {
		return false
	}
	switch st {
	case core.StateRunning, core.StateStarting, core.StatePaused:
		return true
	default:
		return false
	}
}

func (m *model) anyStoppableProcess() bool {
	for _, id := range m.ids {
		if m.stopArmEligible(id) {
			return true
		}
	}
	return false
}

func (m *model) anyKillableProcess() bool {
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok {
			continue
		}
		switch st {
		case core.StateRunning, core.StateStarting, core.StatePaused:
			return true
		default:
		}
	}
	return false
}

func (m *model) shutdownGracefulCmd() tea.Cmd {
	if m.sup == nil {
		return func() tea.Msg { return supShutdownDoneMsg{} }
	}
	var ids []core.ProcessID
	for _, id := range m.ids {
		st, ok := m.store.Get(id)
		if !ok {
			continue
		}
		if st == core.StateRunning || st == core.StatePaused || st == core.StateStarting {
			ids = append(ids, id)
		}
	}
	return supShutdownAllCmd(m.sup, ids)
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(tickCmd(), m.listenCmd())
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case busEventMsg:
		m.consumeBusEvent(core.Event(msg))
		m.drainEvents()
		return m, m.listenCmd()
	case supOpDoneMsg:
		if !(msg.bulk && msg.err == nil) {
			m.appendCtlErrFor(msg.op, msg.id, msg.name, msg.err)
		}
		m.drainEvents()
		m.refreshProcs()
		return m, nil
	case supShutdownDoneMsg:
		return m, tea.Quit
	case supReplaceSaveDoneMsg:
		return m.handleReplaceSaveDone(msg)
	case copyLogDoneMsg:
		if msg.err != nil {
			m.appendToast(ToastErr, "Clipboard: "+msg.err.Error())
		} else {
			m.appendToast(ToastOK, "Copied log window (full lines, plain text)")
		}
		return m, nil
	case tickMsg:
		m.tick++
		m.drainEvents()
		m.refreshProcs()
		if m.pendingQuit && !m.pendingQuitDeadline.IsZero() && time.Now().After(m.pendingQuitDeadline) {
			m.clearPendingQuit()
		}
		if m.pendingDelete && !m.pendingDeleteDeadline.IsZero() && time.Now().After(m.pendingDeleteDeadline) {
			m.clearPendingDelete()
		}
		if m.pendingStop && !m.pendingStopDeadline.IsZero() && time.Now().After(m.pendingStopDeadline) {
			m.clearPendingStop()
		}
		if m.pendingStopAll && !m.pendingStopAllDeadline.IsZero() && time.Now().After(m.pendingStopAllDeadline) {
			m.clearPendingStopAll()
		}
		if !m.toastDeadline.IsZero() && time.Now().After(m.toastDeadline) {
			m.toastText = ""
			m.toastDeadline = time.Time{}
			m.toastKind = ToastNeutral
		}
		if m.overlay == overlayInspector && m.tick%3 == 0 {
			m.refreshInspector()
		}
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampDockScroll(m.dockVisibleCount())
		if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
			m.syncLineFormWidths(m.lineFormInnerW())
		}
		if m.overlay == overlayLogFilter {
			m.syncFilterInpWidth(m.lineFormInnerW())
		}
		// Force a full erase + repaint so terminals (notably tmux) that may have
		// dropped or rewrapped lines around the SIGWINCH come back to a known state.
		// Bubbletea's internal WindowSizeMsg handler only resets the diff cache; it
		// does not clear the screen, so partial corruption can persist after resize.
		return m, tea.ClearScreen
	case tea.KeyMsg:
		// Ctrl+L: force full repaint. Useful when running under tmux and the
		// alt-screen falls out of sync (e.g. after scrollback / copy-mode exit).
		if msg.String() == "ctrl+l" {
			return m, tea.ClearScreen
		}
		if m.overlay == overlayKillSignal {
			if msg.String() == "?" {
				m.helpReturnTo = overlayKillSignal
				m.overlay = overlayHelp
				return m, nil
			}
			switch msg.String() {
			case "ctrl+c", "esc":
				m.closeKillSignalOverlay()
				m.clearPendingQuit()
				return m, nil
			case "enter":
				if cmd := m.applyKillSignalChoice(); cmd != nil {
					return m, cmd
				}
				return m, nil
			}
			switch msg.Type {
			case tea.KeyUp:
				n := len(killSignalMenu())
				if n > 0 {
					m.killSignalSel--
					if m.killSignalSel < 0 {
						m.killSignalSel = n - 1
					}
				}
				return m, nil
			case tea.KeyDown:
				n := len(killSignalMenu())
				if n > 0 {
					m.killSignalSel++
					if m.killSignalSel >= n {
						m.killSignalSel = 0
					}
				}
				return m, nil
			}
			return m, nil
		}
		if m.overlay == overlayLogFilter {
			if msg.String() == "?" {
				m.helpReturnTo = overlayLogFilter
				m.overlay = overlayHelp
				m.filterInp.Blur()
				return m, nil
			}
			switch msg.String() {
			case "ctrl+c":
				m.overlay = overlayNone
				m.clearPendingQuit()
				m.filterInp.Blur()
			case "esc":
				m.overlay = overlayNone
				m.filterInp.Blur()
			case "enter":
				m.applyLogFilter()
				m.filterInp.Blur()
			default:
				var cmd tea.Cmd
				m.filterInp, cmd = m.filterInp.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		// Help opened from the main view (?): any shortcut runs on the main UI; the
		// sheet closes first (Esc, ?, q, Ctrl+c still only dismiss/return as below).
		if m.overlay == overlayHelp && m.helpReturnTo == overlayNone {
			switch msg.String() {
			case "?", "esc", "ctrl+c", "q":
				// handled in the shared key switch below
			default:
				m.overlay = overlayNone
				m.helpReturnTo = overlayNone
				return m.Update(msg)
			}
		}
		if msg.String() != "q" && msg.String() != "ctrl+c" && m.pendingQuit {
			m.clearPendingQuit()
		}
		if msg.String() != "d" && m.pendingDelete {
			m.clearPendingDelete()
		}
		if msg.String() != "t" && m.pendingStop {
			m.clearPendingStop()
		}
		if msg.String() != "T" && m.pendingStopAll {
			m.clearPendingStopAll()
		}
		if m.overlay != overlayAddProcess && m.overlay != overlayEditProcess && m.overlay != overlayKillSignal {
			switch msg.Type {
			case tea.KeyPgUp:
				logH, _ := m.layoutHeights()
				page := max(1, logH)
				m.logScroll += page
				return m, nil
			case tea.KeyPgDown:
				logH, _ := m.layoutHeights()
				page := max(1, logH)
				if m.logScroll >= page {
					m.logScroll -= page
				} else {
					m.logScroll = 0
				}
				return m, nil
			case tea.KeyHome:
				if m.overlay == overlayHelp {
					break
				}
				_ = m.syncLogIndices()
				logH, _ := m.layoutHeights()
				page := max(1, logH)
				if n := len(m.matchedIdx); n > 0 {
					// Align with PgUp page size: show a full viewport of the oldest
					// lines. scrollBack=len-1 would only pass one index into the window.
					m.logScroll = max(0, n-page)
				} else {
					m.logScroll = 0
				}
				m.logHScroll = 0
				return m, nil
			case tea.KeyEnd:
				if m.overlay == overlayHelp {
					break
				}
				m.logScroll = 0
				m.logHScroll = 0
				return m, nil
			case tea.KeyUp:
				if m.overlay == overlayHelp {
					break
				}
				m.logScroll += 3
				return m, nil
			case tea.KeyDown:
				if m.overlay == overlayHelp {
					break
				}
				if m.logScroll >= 3 {
					m.logScroll -= 3
				} else {
					m.logScroll = 0
				}
				return m, nil
			case tea.KeyShiftUp:
				if m.overlay == overlayHelp {
					break
				}
				if m.selected > 0 {
					m.selected--
					m.clampDockScroll(m.dockVisibleCount())
				}
				return m, nil
			case tea.KeyShiftDown:
				if m.overlay == overlayHelp {
					break
				}
				if m.selected < len(m.processes)-1 {
					m.selected++
					m.clampDockScroll(m.dockVisibleCount())
				}
				return m, nil
			case tea.KeyLeft:
				if m.overlay == overlayHelp {
					break
				}
				if !m.logWordWrap {
					if m.logHScroll >= 3 {
						m.logHScroll -= 3
					} else {
						m.logHScroll = 0
					}
				}
				return m, nil
			case tea.KeyRight:
				if m.overlay == overlayHelp {
					break
				}
				if !m.logWordWrap {
					m.logHScroll += 3
				}
				return m, nil
			}
		}
		if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
			edit := m.overlay == overlayEditProcess
			if msg.String() == "?" {
				m.helpReturnTo = m.overlay
				m.overlay = overlayHelp
				m.blurAllLineInputs()
				return m, nil
			}
			switch msg.String() {
			case "ctrl+c":
				m.resetLineOverlay()
				m.clearPendingQuit()
				return m, nil
			case "esc":
				m.resetLineOverlay()
				return m, nil
			case "enter":
				if edit {
					if cmd := m.tryEditProcess(); cmd != nil {
						return m, cmd
					}
				} else {
					m.tryAddProcess()
				}
				// Keep cursor blink / focus after a failed save (textinput stops scheduling blink without a Focus cmd).
				if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
					return m, m.refocusLineFormCurrent()
				}
				return m, nil
			case "tab", "shift+tab":
				m.modalErr = ""
				if m.lineOverlayField == lineFormNameField {
					m.lineOverlayField = lineFormCmdField
				} else {
					m.lineOverlayField = lineFormNameField
				}
				m.syncLineFormWidths(m.lineFormInnerW())
				return m, m.refocusLineFormCurrent()
			}
			if msg.Type == tea.KeyUp || msg.Type == tea.KeyDown {
				m.modalErr = ""
				if m.lineOverlayField == lineFormNameField {
					m.lineOverlayField = lineFormCmdField
				} else {
					m.lineOverlayField = lineFormNameField
				}
				m.syncLineFormWidths(m.lineFormInnerW())
				return m, m.refocusLineFormCurrent()
			}
			m.modalErr = ""
			cmd := m.dispatchLineFormUpdate(msg, edit)
			return m, cmd
		}
		switch msg.String() {
		case "ctrl+c", "q":
			switch m.overlay {
			case overlayHelp:
				m.overlay = m.helpReturnTo
				m.helpReturnTo = overlayNone
				m.clearPendingQuit()
				m.clearPendingDelete()
				m.clearPendingStop()
				m.clearPendingStopAll()
				if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
					m.syncLineFormWidths(m.lineFormInnerW())
					return m, m.refocusLineFormCurrent()
				}
				if m.overlay == overlayLogFilter {
					m.syncFilterInpWidth(m.lineFormInnerW())
					return m, m.filterInp.Focus()
				}
				if m.overlay == overlayKillSignal {
					return m, nil
				}
				return m, nil
			case overlayNone:
				if m.pendingQuit && !m.pendingQuitDeadline.IsZero() && time.Now().Before(m.pendingQuitDeadline) {
					return m, m.shutdownGracefulCmd()
				}
				m.clearPendingDelete()
				m.clearPendingStop()
				m.clearPendingStopAll()
				m.pendingQuit = true
				m.pendingQuitDeadline = time.Now().Add(pendingConfirmWindow)
				return m, nil
			default:
				if m.overlay == overlayKillSignal {
					m.closeKillSignalOverlay()
				} else {
					m.overlay = overlayNone
				}
				m.clearPendingQuit()
				m.clearPendingDelete()
				m.clearPendingStop()
				m.clearPendingStopAll()
				return m, nil
			}
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
				if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
					m.syncLineFormWidths(m.lineFormInnerW())
					return m, m.refocusLineFormCurrent()
				}
				if m.overlay == overlayLogFilter {
					m.syncFilterInpWidth(m.lineFormInnerW())
					return m, m.filterInp.Focus()
				}
				if m.overlay == overlayKillSignal {
					return m, nil
				}
			} else {
				prev := m.overlay
				m.helpReturnTo = prev
				m.overlay = overlayHelp
				if prev == overlayAddProcess || prev == overlayEditProcess {
					m.blurAllLineInputs()
				}
				if prev == overlayLogFilter {
					m.filterInp.Blur()
				}
			}
		case "esc":
			if m.overlay == overlayHelp {
				m.overlay = m.helpReturnTo
				m.helpReturnTo = overlayNone
				if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
					m.syncLineFormWidths(m.lineFormInnerW())
					return m, m.refocusLineFormCurrent()
				}
				if m.overlay == overlayLogFilter {
					m.syncFilterInpWidth(m.lineFormInnerW())
					return m, m.filterInp.Focus()
				}
				if m.overlay == overlayKillSignal {
					return m, nil
				}
			} else if m.overlay != overlayNone {
				if m.overlay == overlayKillSignal {
					m.closeKillSignalOverlay()
				} else {
					m.overlay = overlayNone
				}
			} else {
				if m.pendingQuit {
					m.clearPendingQuit()
				}
				if m.pendingDelete {
					m.clearPendingDelete()
				}
				if m.pendingStop {
					m.clearPendingStop()
				}
				if m.pendingStopAll {
					m.clearPendingStopAll()
				}
			}
		case "o":
			if m.overlay == overlayHelp {
				break
			}
			m.showStdout = !m.showStdout
			m.forceLogRebuild()
		case "e":
			if m.overlay == overlayHelp {
				break
			}
			m.showStderr = !m.showStderr
			m.forceLogRebuild()
		case "w":
			if m.overlay == overlayHelp {
				break
			}
			m.logWordWrap = !m.logWordWrap
			m.logHScroll = 0
			if m.logWordWrap {
				m.appendToast(ToastOK, "Log word wrap on")
			} else {
				m.appendToast(ToastOK, "Log word wrap off")
			}
		case "c":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay != overlayNone {
				break
			}
			logH, _ := m.layoutHeights()
			return m, runCopyLogPlainCmd(m.plainLogCopyText(logH))
		case "P":
			if m.overlay == overlayHelp {
				break
			}
			m.logTimePrec = m.logTimePrec.prev()
		case "p":
			if m.overlay == overlayHelp {
				break
			}
			m.logTimePrec = m.logTimePrec.next()
		case "tab":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay != overlayNone {
				break
			}
			n := len(m.processes)
			if n == 0 {
				break
			}
			m.selected = (m.selected + 1) % n
			m.clampDockScroll(m.dockVisibleCount())
		case "shift+tab":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay != overlayNone {
				break
			}
			n := len(m.processes)
			if n == 0 {
				break
			}
			m.selected = (m.selected - 1 + n) % n
			m.clampDockScroll(m.dockVisibleCount())
		case "/":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayKillSignal {
				break
			}
			m.overlay = overlayLogFilter
			m.filterInp.SetValue(m.filterPattern)
			m.syncFilterInpWidth(m.lineFormInnerW())
			return m, m.filterInp.Focus()
		case "n":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			m.overlay = overlayAddProcess
			m.modalErr = ""
			m.lineOverlayField = lineFormNameField
			m.addNameInp.Reset()
			m.addCmdInp.Reset()
			m.syncLineFormWidths(m.lineFormInnerW())
			return m, m.refocusLineFormCurrent()
		case "d":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			if m.sup == nil {
				break
			}
			id := m.currentID()
			if m.pendingDelete {
				if time.Now().After(m.pendingDeleteDeadline) || m.pendingDeleteID != id {
					m.clearPendingDelete()
				}
			}
			if m.pendingDelete && m.pendingDeleteID == id && time.Now().Before(m.pendingDeleteDeadline) {
				m.clearPendingDelete()
				m.tryDeleteProcess()
				break
			}
			if err := m.deletePrevalidate(id); err != nil {
				m.appendToast(ToastErr, "Delete: "+err.Error())
				break
			}
			m.clearPendingQuit()
			m.clearPendingStop()
			m.clearPendingStopAll()
			m.pendingDelete = true
			m.pendingDeleteDeadline = time.Now().Add(pendingConfirmWindow)
			m.pendingDeleteID = id
		case "T":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			if m.sup == nil {
				break
			}
			if m.pendingStopAll {
				if time.Now().After(m.pendingStopAllDeadline) {
					m.clearPendingStopAll()
				}
			}
			if m.pendingStopAll && time.Now().Before(m.pendingStopAllDeadline) {
				m.clearPendingStopAll()
				m.clearPendingStop()
				m.clearPendingQuit()
				m.clearPendingDelete()
				if cmd := m.stopAllGracefulCmd(); cmd != nil {
					return m, cmd
				}
				break
			}
			if !m.anyStoppableProcess() {
				m.appendToast(ToastErr, "Terminate all: nothing running to terminate")
				break
			}
			m.clearPendingQuit()
			m.clearPendingDelete()
			m.clearPendingStop()
			m.pendingStopAll = true
			m.pendingStopAllDeadline = time.Now().Add(pendingConfirmWindow)
		case "t":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			if m.sup == nil {
				break
			}
			id := m.currentID()
			if id == "" {
				m.appendToast(ToastErr, "Terminate: no process selected")
				break
			}
			if m.pendingStop {
				if time.Now().After(m.pendingStopDeadline) || m.pendingStopID != id {
					m.clearPendingStop()
				}
			}
			if m.pendingStop && m.pendingStopID == id && time.Now().Before(m.pendingStopDeadline) {
				m.clearPendingStop()
				m.clearPendingStopAll()
				m.clearPendingQuit()
				m.clearPendingDelete()
				if cmd := supStopCmd(m.sup, id, m.nameForID(id)); cmd != nil {
					return m, cmd
				}
				break
			}
			if !m.stopArmEligible(id) {
				m.appendToast(ToastErr, "Terminate: nothing to terminate in this state")
				break
			}
			m.clearPendingQuit()
			m.clearPendingDelete()
			m.clearPendingStopAll()
			m.pendingStop = true
			m.pendingStopDeadline = time.Now().Add(pendingConfirmWindow)
			m.pendingStopID = id
		case "enter":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
				break
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			if cmd := m.openEditProcessFromMain(); cmd != nil {
				return m, cmd
			}
		case "i":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			} else if m.currentID() != "" {
				m.overlay = overlayInspector
				m.refreshInspector()
			}
		case "r":
			if m.overlay == overlayInspector {
				m.refreshInspector()
				break
			}
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			if m.sup == nil {
				break
			}
			if id := m.currentID(); id != "" {
				if cmd := supStartCmd(m.sup, id, m.nameForID(id)); cmd != nil {
					return m, cmd
				}
			}
		case "k":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			if m.sup == nil {
				break
			}
			id := m.currentID()
			if id == "" {
				m.appendToast(ToastErr, "Signal: no process selected")
				break
			}
			m.clearPendingQuit()
			m.clearPendingDelete()
			m.clearPendingStop()
			m.clearPendingStopAll()
			m.killSignalBulkAll = false
			m.killSignalTargetID = id
			m.killSignalSel = 0
			m.overlay = overlayKillSignal
		case "K":
			if m.overlay == overlayHelp {
				break
			}
			if m.overlay == overlayInspector {
				m.overlay = overlayNone
			}
			if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal {
				break
			}
			if m.sup == nil {
				break
			}
			if !m.anyKillableProcess() {
				m.appendToast(ToastErr, "Signal: nothing running to target")
				break
			}
			m.clearPendingQuit()
			m.clearPendingDelete()
			m.clearPendingStop()
			m.clearPendingStopAll()
			m.killSignalBulkAll = true
			m.killSignalTargetID = ""
			m.killSignalSel = 0
			m.overlay = overlayKillSignal
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
			if m.sup == nil {
				break
			}
			switch msg.String() {
			case "R":
				if cmd := m.startAllGracefulCmd(); cmd != nil {
					return m, cmd
				}
			case "Z":
				if cmd := m.pauseAllGracefulCmd(); cmd != nil {
					return m, cmd
				}
			case "V":
				if cmd := m.continueAllGracefulCmd(); cmd != nil {
					return m, cmd
				}
			case "Y":
				if cmd := m.restartAllGracefulCmd(); cmd != nil {
					return m, cmd
				}
			case "z":
				if id := m.currentID(); id != "" {
					if cmd := supPauseCmd(m.sup, id, m.nameForID(id)); cmd != nil {
						return m, cmd
					}
				}
			case "v":
				if id := m.currentID(); id != "" {
					if cmd := supContinueCmd(m.sup, id, m.nameForID(id)); cmd != nil {
						return m, cmd
					}
				}
			case "y":
				if id := m.currentID(); id != "" {
					if cmd := supRestartCmd(m.sup, id, m.nameForID(id)); cmd != nil {
						return m, cmd
					}
				}
			}
		}
	}

	return m, nil
}

func (m *model) View() string {
	if m.width < 40 || m.height < 8 {
		return "Terminal too small (min 40×8). Press q again to quit."
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
	return lipgloss.JoinVertical(lipgloss.Left, logBlock, footer)
}

func (m *model) renderFooter() string {
	w := m.width
	if w < 1 {
		w = 1
	}
	var parts []string
	var trail string
	switch m.overlay {
	case overlayHelp:
		if m.helpReturnTo == overlayNone {
			parts = []string{"Esc ? close"}
		} else {
			parts = []string{"Esc ? return"}
		}
	case overlayInspector:
		parts = []string{"Esc or Enter closes", "r refresh"}
	case overlayAddProcess:
		parts = []string{"Esc cancels", "Tab field", "Enter registers · r runs"}
	case overlayEditProcess:
		parts = []string{"Esc cancels", "Tab field", "Enter saves · r runs"}
	case overlayLogFilter:
		parts = []string{"Esc or Ctrl+c cancels", "Enter apply"}
	case overlayKillSignal:
		parts = []string{"↑↓ choose signal", "Enter send", "Esc cancels"}
	default:
		if m.pendingQuit || m.pendingDelete || m.pendingStop || m.pendingStopAll {
			if m.pendingQuit {
				parts = append(parts, "Press q or Ctrl+c again to quit")
			}
			if m.pendingDelete {
				parts = append(parts, "Press d again to confirm delete")
			}
			if m.pendingStop {
				parts = append(parts, "Press t to confirm terminate")
			}
			if m.pendingStopAll {
				parts = append(parts, "Press T to confirm terminate all")
			}
			s := joinFooterImportantTrail(parts, "", w)
			return StyleFooterPending(padRight(truncate(s, w), w))
		}
		if !m.toastDeadline.IsZero() && time.Now().Before(m.toastDeadline) && m.toastText != "" {
			s := joinFooterImportantTrail([]string{m.toastText}, "", w)
			return StyleFooterToast(padRight(truncate(s, w), w), m.toastKind)
		}
		trail = "? help"
		parts = append(parts,
			"q quit",
			"n new",
			"r run",
			"t terminate",
			"k signal",
			"d delete",
			"i inspect",
			"/ filter",
			"Tab dock",
		)
	}
	s := joinFooterImportantTrail(parts, trail, w)
	return StyleFooterMuted(padRight(truncate(s, w), w))
}

// logViewportStyledLines returns the same log rows shown in the main pane (styled),
// including word wrap and horizontal pan, before joining for View().
func (m *model) logViewportStyledLines(bodyH int) []string {
	w := m.width
	if w < 1 {
		w = 1
	}
	h := bodyH
	if h < 1 {
		h = 1
	}

	var lines []string
	if m.logWordWrap {
		m.logHScroll = 0
		if err := m.syncLogIndices(); err != nil {
			lines = neutralPlaceholders(h)
		} else {
			m.clampLogScroll()
			nameCol := dockNameColumnWidth(m.processes, 4, 32)
			var err error
			lines, err = BuildWrappedWindowLinesFromIndices(m.slog, m.matchedIdx, m.logScroll, h, w, m.logTimePrec, m.dockIDStrings(), nameCol)
			if err != nil {
				lines = neutralPlaceholders(h)
			}
		}
		for i := range lines {
			lines[i] = padToCellWidth(lines[i], w)
		}
	} else {
		lines = m.composeLines(h)
		m.clampLogHScroll(lines, w)
		for i := range lines {
			lines[i] = padToCellWidth(ansi.Cut(lines[i], m.logHScroll, m.logHScroll+w), w)
		}
	}
	return lines
}

func (m *model) renderBody(bodyH int) string {
	lines := m.logViewportStyledLines(bodyH)
	return strings.Join(lines, "\n")
}

// plainLogCopyText returns plain text for the same matched-record window as the log
// pane (scroll position and height), but each line is the full logical row from disk
// (not horizontally cut or wrapped for display). ANSI is stripped.
func (m *model) plainLogCopyText(bodyH int) string {
	if err := m.syncLogIndices(); err != nil {
		return ""
	}
	m.clampLogScroll()
	nameCol := dockNameColumnWidth(m.processes, 4, 32)
	lines, err := BuildWindowLinesFromIndices(m.slog, m.matchedIdx, m.logScroll, bodyH, m.logTimePrec, m.dockIDStrings(), nameCol)
	if err != nil {
		return ""
	}
	for i := range lines {
		lines[i] = strings.TrimSpace(ansi.Strip(lines[i]))
	}
	return strings.Join(lines, "\n")
}

// dockIDStrings returns all slot IDs in stable order (including tombstones) for log colors.
func (m *model) dockIDStrings() []string {
	if len(m.slots) == 0 {
		return nil
	}
	out := make([]string, len(m.slots))
	for i := range m.slots {
		out[i] = string(m.slots[i].ID)
	}
	return out
}

// clampLogScroll keeps scroll offset within matched lines so we never index past
// the log (BuildWindowLinesFromIndices would otherwise show empty "past end" rows).
func (m *model) clampLogScroll() {
	n := len(m.matchedIdx)
	if n == 0 {
		m.logScroll = 0
		return
	}
	maxSB := n - 1
	if m.logScroll > maxSB {
		m.logScroll = maxSB
	}
	if m.logScroll < 0 {
		m.logScroll = 0
	}
}

// clampLogHScroll keeps horizontal pan within the widest visible log line.
func (m *model) clampLogHScroll(lines []string, viewW int) {
	if m.logWordWrap {
		m.logHScroll = 0
		return
	}
	if viewW < 1 {
		m.logHScroll = 0
		return
	}
	maxW := 0
	for _, ln := range lines {
		if sw := ansi.StringWidth(ln); sw > maxW {
			maxW = sw
		}
	}
	maxPan := maxW - viewW
	if maxPan < 0 {
		maxPan = 0
	}
	if m.logHScroll > maxPan {
		m.logHScroll = maxPan
	}
	if m.logHScroll < 0 {
		m.logHScroll = 0
	}
}

func (m *model) composeLines(n int) []string {
	if err := m.syncLogIndices(); err != nil {
		return neutralPlaceholders(n)
	}
	m.clampLogScroll()
	nameCol := dockNameColumnWidth(m.processes, 4, 32)
	lines, err := BuildWindowLinesFromIndices(m.slog, m.matchedIdx, m.logScroll, n, m.logTimePrec, m.dockIDStrings(), nameCol)
	if err != nil {
		return neutralPlaceholders(n)
	}
	return lines
}

// helpOverlayContent builds the help modal when m.overlay == overlayHelp; which
// sheet to show depends on helpReturnTo (the overlay ? was pressed from).
func (m *model) helpOverlayContent() (title string, bodyLines []string) {
	switch m.helpReturnTo {
	case overlayInspector:
		title = "Inspector help"
		bodyLines = []string{
			"Live metrics for the selected dock process (not a separate log view).",
			"PID, recent CPU sample, RSS, threads, open file descriptors, command line.",
			"",
			"  r           refresh snapshot",
			"  Esc Enter   close inspector",
			"  ? Esc       close this help",
			"",
			"Session-wide keys (dock, log, process controls): close inspector first, then ? on the main view.",
		}
	case overlayKillSignal:
		title = "Send signal help"
		bodyLines = []string{
			"Pick how to end or interrupt processes. k targets the selected slot; K targets every running slot.",
			"Graceful terminate matches a full Terminate (SIGTERM, wait, then SIGKILL if needed on unix).",
			"Other POSIX signals are sent to the process group; USR1/USR2/WINCH usually leave the child running (see meta lines in the log).",
			"",
			"  ↑ ↓         move highlight",
			"  Enter       send the highlighted choice",
			"  Esc Ctrl+c  close without sending",
			"  ? Esc       close this help",
			"",
			"Other imux shortcuts: close this overlay, then ? on the main view.",
		}
	case overlayLogFilter:
		title = "Log filter help"
		bodyLines = []string{
			"Restricts which merged log lines match. Empty clears the filter.",
			"",
			"  Enter       apply and close  ·  Esc Ctrl+c cancel",
			"  ? Esc       close this help",
			"  ←→ Home/End move in the pattern field",
			"",
			"Other imux shortcuts: close this overlay, then ? on the main view.",
		}
	case overlayAddProcess:
		title = "New process help"
		bodyLines = []string{
			"Adds a dock slot wrapped like imux run (sh -c … or cmd /C … on Windows).",
			"",
			"  Tab         move between name and command fields",
			"  Enter       register (still stopped; press r on the dock to run)",
			"  Esc Ctrl+c  discard and close",
			"  ? Esc       close this help",
			"",
			"Display names must be unique across slots (case-insensitive).",
		}
	case overlayEditProcess:
		title = "Edit process help"
		bodyLines = []string{
			"Change display name and shell command for this slot.",
			"If the process is running, Enter terminates it for you, then saves (same as replace-then-save).",
			"",
			"  Tab         move between name and command fields",
			"  Enter       save (press r on the dock when you want it running again)",
			"  Esc Ctrl+c  discard and close",
			"  ? Esc       close this help",
			"",
			"Display names must be unique across slots (case-insensitive).",
		}
	default:
		title = "Help"
		proc := strings.TrimSpace(m.currentName())
		id := string(m.currentID())
		var focusLine string
		switch {
		case proc != "" && id != "" && !strings.EqualFold(proc, id):
			focusLine = fmt.Sprintf("Focus: %s (%s)", proc, id)
		case proc != "":
			focusLine = "Focus: " + proc
		case id != "":
			focusLine = "Focus: " + id
		default:
			focusLine = "Focus: —"
		}
		bodyLines = []string{
			"One merged log for all processes (dock selection does not swap the log).",
			"Log lines are stored on disk (unlinked temp); use imux --tee for a persisted copy.",
			"Scroll the log with ↑ ↓ PgUp PgDn and Home/End; drag with the mouse to select like any terminal text.",
			"From this sheet (main view): any other key runs that shortcut and closes help.",
			"",
			"Keys:",
			"  Tab Shift+Tab move dock selection (Shift+↑↓ same as on dock)",
			"  1-9           jump to process slot (first nine)",
			"  r t z v y     run / terminate / pause / continue / restart (selected)",
			"  k             signal menu for selected slot, then enter",
			"  K             same menu for every running process, then enter",
			"  R T Z V Y     bulk run / terminate all / pause / continue / restart",
			"  , or .        previous / next process (same as Tab / Shift+Tab)",
			"  n             new",
			"  i             inspector",
			"  enter         edit name + command for the selected slot",
			"  d             delete slot",
			"  o e           toggle stdout / stderr",
			"  w             toggle log word wrap",
			"  c             copy log window (full lines)",
			"  p P           log time precision: p next, P prev (off → s → ms → us)",
			"  /             edit log filter",
			"  ? Esc         help · close overlay",
			"  q Ctrl+c      quit",
			"",
			focusLine,
		}
	}
	return title, bodyLines
}

// helpStructuredRow is true for key-binding rows and the "Keys:" header so help
// prose can be merged and wrapped as flowing paragraphs without breaking at each
// source string boundary.
func helpStructuredRow(s string) bool {
	if strings.HasPrefix(s, "  ") {
		return true
	}
	return s == "Keys:"
}

// wrapModalLines word-wraps to w terminal cells. When mergeHelpProse is true
// (help overlay only), consecutive non-empty lines that are not helpStructuredRow
// are joined with spaces and wrapped as one paragraph so breaks follow the pane
// width instead of artificial line breaks in the source.
func wrapModalLines(lines []string, w int, mergeHelpProse bool) []string {
	if w < 4 {
		w = 4
	}
	if !mergeHelpProse {
		var out []string
		for _, ln := range lines {
			if ln == "" {
				out = append(out, "")
				continue
			}
			wrapped := ansi.Wrap(ln, w, "")
			for _, seg := range strings.Split(wrapped, "\n") {
				out = append(out, seg)
			}
		}
		return out
	}

	var out []string
	var prose []string
	flushProse := func() {
		if len(prose) == 0 {
			return
		}
		joined := strings.Join(prose, " ")
		prose = prose[:0]
		wrapped := ansi.Wrap(joined, w, "")
		for _, seg := range strings.Split(wrapped, "\n") {
			out = append(out, seg)
		}
	}
	for _, ln := range lines {
		if ln == "" {
			flushProse()
			out = append(out, "")
			continue
		}
		if helpStructuredRow(ln) {
			flushProse()
			wrapped := ansi.Wrap(ln, w, "")
			for _, seg := range strings.Split(wrapped, "\n") {
				out = append(out, seg)
			}
			continue
		}
		prose = append(prose, ln)
	}
	flushProse()
	return out
}

func (m *model) renderModal() string {
	maxW := min(56, m.width-6)
	if m.overlay == overlayHelp {
		maxW = min(72, m.width-4)
	}
	if m.overlay == overlayInspector {
		maxW = min(72, m.width-4)
	}
	if m.overlay == overlayAddProcess || m.overlay == overlayEditProcess {
		maxW = min(72, m.width-4)
	}
	if m.overlay == overlayLogFilter {
		maxW = min(72, m.width-4)
	}
	if m.overlay == overlayKillSignal {
		maxW = min(72, m.width-4)
	}
	if maxW < 24 {
		maxW = 24
	}

	innerW := maxW - 2
	if innerW < 4 {
		innerW = 4
	}

	var title string
	var bodyLines []string
	switch m.overlay {
	case overlayHelp:
		title, bodyLines = m.helpOverlayContent()
	case overlayInspector:
		title = "Inspector"
		bodyLines = append(append([]string(nil), m.inspectLines...), "", "Esc or Enter closes · r refresh · ? panel help")
	case overlayAddProcess:
		title = "New process"
		bodyLines = appendModalSaveErr(
			append([]string{"Wrapped like imux run (sh -c or cmd /C)."},
				m.lineFormModalLines(m.lineFormInnerW(), false, "Esc cancels · Enter registers · Tab switches field · r runs")...),
			innerW, m.modalErr)
	case overlayEditProcess:
		title = "Edit process"
		bodyLines = appendModalSaveErr(
			append([]string{fmt.Sprintf("id %s — same slot.", m.editTargetID)},
				m.lineFormModalLines(m.lineFormInnerW(), true, "Esc cancels · Enter saves · Tab switches field · r runs when stopped")...),
			innerW, m.modalErr)
	case overlayLogFilter:
		title = "Log filter"
		m.syncFilterInpWidth(innerW)
		bodyLines = []string{
			"CLI: --log-filter 're:…' or a bare pattern. Empty clears.",
			"",
			m.filterInp.View(),
			"",
			"Esc or Ctrl+c cancel · Enter apply",
		}
	case overlayKillSignal:
		if m.killSignalBulkAll {
			title = "Signal → all running"
		} else {
			title = "Send signal"
		}
		id := m.killSignalTargetID
		bodyLines = killSignalModalLines(m.killSignalSel, m.killSignalBulkAll, id, m.nameForID(id), m.killableRunningCount())
	default:
		title = ""
		bodyLines = nil
	}

	if title == "" && len(bodyLines) == 0 {
		return ""
	}
	if bodyLines == nil {
		bodyLines = []string{}
	}

	bodyLines = wrapModalLines(bodyLines, innerW, m.overlay == overlayHelp)

	maxOuter := m.height - 4
	if maxOuter < 7 {
		maxOuter = 7
	}
	maxInner := maxOuter - 2
	// Inner rows = title + blank separator + body (header rendered separately below).
	innerLines := min(maxInner, max(3, 2+len(bodyLines)))

	formOverlay := m.overlay == overlayAddProcess || m.overlay == overlayEditProcess || m.overlay == overlayLogFilter || m.overlay == overlayKillSignal

	bodyCapacity := innerLines - 2
	if bodyCapacity < 1 {
		bodyCapacity = 1
	}
	if len(bodyLines) > bodyCapacity {
		if formOverlay {
			// Never drop the bottom of a form: keep the tail (fields + footer + error) so edits are still visible.
			if bodyCapacity <= 1 {
				last := bodyLines[len(bodyLines)-1]
				bodyLines = []string{padToCellWidth(ansi.Truncate(last, innerW, "…"), innerW)}
			} else {
				keep := bodyCapacity - 1
				tail := bodyLines[len(bodyLines)-keep:]
				warn := padToCellWidth("… top clipped — enlarge terminal or Esc", innerW)
				bodyLines = append([]string{warn}, tail...)
			}
		} else {
			bodyLines = bodyLines[:max(0, bodyCapacity-1)]
			bodyLines = append(bodyLines, padToCellWidth("… (terminal too short — widen or close)", innerW))
		}
	}
	for len(bodyLines) < bodyCapacity {
		bodyLines = append(bodyLines, "")
	}
	for i := range bodyLines {
		bodyLines[i] = padToCellWidth(bodyLines[i], innerW)
	}

	header := padToCellWidth(title, innerW)
	sep := padToCellWidth("", innerW)
	all := append([]string{header, sep}, bodyLines...)

	// Modal: no fill color — use the terminal default background/foreground so it matches the rest of the UI.
	style := lipgloss.NewStyle().
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245")).
		Width(innerW)

	return style.Render(strings.Join(all, "\n"))
}

func truncate(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= maxWidth {
		return s
	}
	if maxWidth == 1 {
		return "…"
	}
	return ansi.Truncate(s, maxWidth, "…")
}

func padRight(s string, width int) string {
	visible := lipgloss.Width(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
}

// ttyProgramOpts returns extra tea options when stdin and/or stdout are not terminals
// but the session still has a usable controlling terminal at /dev/tty.
func ttyProgramOpts() (opts []tea.ProgramOption, cleanup func()) {
	var tty *os.File
	cleanup = func() {
		if tty != nil {
			_ = tty.Close()
			tty = nil
		}
	}
	if runtime.GOOS == "windows" {
		return nil, cleanup
	}
	inTTY := xterm.IsTerminal(os.Stdin.Fd())
	outTTY := xterm.IsTerminal(os.Stdout.Fd())
	if inTTY && outTTY {
		return nil, cleanup
	}
	var err error
	tty, err = os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		tty, err = os.Open("/dev/tty")
	}
	if err != nil || tty == nil || !xterm.IsTerminal(tty.Fd()) {
		if tty != nil {
			_ = tty.Close()
			tty = nil
		}
		return nil, cleanup
	}
	if !inTTY && !outTTY {
		return append(opts, tea.WithInput(tty), tea.WithOutput(tty)), cleanup
	}
	if !inTTY {
		opts = append(opts, tea.WithInput(tty))
	}
	if !outTTY {
		opts = append(opts, tea.WithOutput(tty))
	}
	return opts, cleanup
}

// Run launches the alt-screen Bubble Tea application.
func Run(opts Options) error {
	m, err := newModel(opts)
	if err != nil {
		return err
	}
	defer m.dispose()

	ttyOpts, cleanupTTY := ttyProgramOpts()
	defer cleanupTTY()

	base := []tea.ProgramOption{tea.WithAltScreen()}
	p := tea.NewProgram(m, append(base, ttyOpts...)...)
	_, err = p.Run()
	if err != nil && strings.Contains(err.Error(), "could not open a new TTY") {
		return fmt.Errorf("%w\n\nThe TUI needs a real terminal (stdin/stdout on a TTY, or a working /dev/tty).", err)
	}
	return err
}
