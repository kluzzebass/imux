package tui

import (
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"imux/internal/sessionlog"
)

// maxLogScanLines caps backwards scan cost when rebuilding the match index.
const maxLogScanLines = 100000

// Stream marks sit immediately before the grey "[name]" block: blank for stdout, yellow triangle for stderr.
const (
	logStreamMarkStdout = " "
	logStreamMarkStderr = "▲"
)

// logPalette is cycled by dock slot order so concurrent processes never share a color
// until there are more processes than colors (hash fallback only when id is unknown).
var logPalette = []lipgloss.Color{
	"39", "208", "214", "183", "162", "117", "141", "111",
	"6", "5", "3", "2", "4", "1",
}

func colorStyleForProcess(name, id string, dockOrder []string) lipgloss.Style {
	n := len(logPalette)
	if n == 0 {
		return lipgloss.NewStyle()
	}
	if id != "" && len(dockOrder) > 0 {
		for i, dockID := range dockOrder {
			if dockID == id {
				c := logPalette[i%n]
				return lipgloss.NewStyle().Foreground(c)
			}
		}
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(id))
	c := logPalette[h.Sum32()%uint32(n)]
	return lipgloss.NewStyle().Foreground(c)
}

func streamMarkForRecord(rec sessionlog.Record) string {
	switch rec.K {
	case sessionlog.KindStdout:
		return logStreamMarkStdout
	case sessionlog.KindStderr:
		return logStreamMarkStderr
	default:
		tag := string(rec.K)
		if tag == "" {
			return "?"
		}
		return tag
	}
}

func flatRecord(rec sessionlog.Record) string {
	who := rec.Name
	if who == "" {
		who = rec.ID
	}
	if who == "" {
		who = "?"
	}
	return fmt.Sprintf("%s[%s] %s", streamMarkForRecord(rec), who, rec.Msg)
}

func passesStreamToggles(rec sessionlog.Record, showStdout, showStderr bool) bool {
	switch rec.K {
	case sessionlog.KindStdout:
		return showStdout
	case sessionlog.KindStderr:
		return showStderr
	default:
		return false
	}
}

func isChildStream(rec sessionlog.Record) bool {
	return rec.K == sessionlog.KindStdout || rec.K == sessionlog.KindStderr
}

type compiledFilter struct {
	re *regexp.Regexp
}

func compileFilter(pattern string) (*compiledFilter, error) {
	if pattern == "" {
		return nil, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &compiledFilter{re: re}, nil
}

func (c *compiledFilter) match(flat string) bool {
	if c == nil {
		return true
	}
	return c.re.MatchString(flat)
}

// MatchLineIndices scans recent log lines and returns matching line indices
// newest-first (index 0 is the chronologically newest matching line).
func MatchLineIndices(slog *sessionlog.SessionLog, filter *compiledFilter, showStdout, showStderr bool) ([]int64, error) {
	if slog == nil {
		return nil, nil
	}
	n, err := slog.LineCount()
	if err != nil {
		return nil, err
	}
	var out []int64
	var scanned int64
	for i := n - 1; i >= 0 && scanned < maxLogScanLines; i-- {
		scanned++
		rec, err := slog.ReadLine(i)
		if err != nil {
			return nil, err
		}
		if !isChildStream(rec) {
			continue
		}
		if !passesStreamToggles(rec, showStdout, showStderr) {
			continue
		}
		if !filter.match(flatRecord(rec)) {
			continue
		}
		out = append(out, i)
	}
	return out, nil
}

// dockNameColumnWidth returns a fixed display width for the process name segment
// in log prefixes so columns align across lines (clamped to [minW, maxW]).
func dockNameColumnWidth(names []string, minW, maxW int) int {
	w := minW
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if lw := lipgloss.Width(n); lw > w {
			w = lw
		}
	}
	if w > maxW {
		w = maxW
	}
	return w
}

// padLogNamePlain left-pads the name (right-aligns in the cell) so timestamps
// and messages line up after the closing bracket.
func padLogNamePlain(s string, colW int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		s = "?"
	}
	if colW < 1 {
		return s
	}
	if lipgloss.Width(s) > colW {
		return ansi.Truncate(s, colW, "…")
	}
	return strings.Repeat(" ", colW-lipgloss.Width(s)) + s
}

// BuildWindowLinesFromIndices renders up to h lines (oldest at [0]) using precomputed
// match indices (newest-first). Only reads at most h lines from disk.
func BuildWindowLinesFromIndices(
	slog *sessionlog.SessionLog,
	indices []int64,
	scrollBack, h int,
	timePrec logTimePrecision,
	dockIDOrder []string,
	nameColW int,
) ([]string, error) {
	if h < 1 {
		return nil, nil
	}
	if slog == nil {
		return neutralPlaceholders(h), nil
	}
	n, err := slog.LineCount()
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return neutralPlaceholders(h), nil
	}
	if len(indices) == 0 {
		return neutralPlaceholders(h), nil
	}
	if scrollBack >= len(indices) {
		return scrolledPastPlaceholders(h), nil
	}
	end := scrollBack + h
	if end > len(indices) {
		end = len(indices)
	}
	slice := indices[scrollBack:end]
	out := make([]string, 0, h)
	for i := len(slice) - 1; i >= 0; i-- {
		rec, err := slog.ReadLine(slice[i])
		if err != nil {
			return nil, err
		}
		out = append(out, formatStyledLogLine(rec, timePrec, dockIDOrder, nameColW))
	}
	for len(out) < h {
		out = append([]string{""}, out...)
	}
	if len(out) > h {
		out = out[len(out)-h:]
	}
	return out, nil
}

func formatStyledLogLine(rec sessionlog.Record, timePrec logTimePrecision, dockIDOrder []string, nameColW int) string {
	who := rec.Name
	if who == "" {
		who = string(rec.ID)
	}
	if who == "" {
		who = "?"
	}
	whoCell := who
	if nameColW > 0 {
		whoCell = padLogNamePlain(who, nameColW)
	}
	st := colorStyleForProcess(rec.Name, rec.ID, dockIDOrder)
	grey := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	yellow := lipgloss.NewStyle().Foreground(lipgloss.Color("220"))

	var prefStyled string
	switch rec.K {
	case sessionlog.KindStdout:
		prefStyled = logStreamMarkStdout + grey.Render("[") + st.Render(whoCell) + grey.Render("]")
	case sessionlog.KindStderr:
		prefStyled = yellow.Render(logStreamMarkStderr) + grey.Render("[") + st.Render(whoCell) + grey.Render("]")
	default:
		mark := streamMarkForRecord(rec)
		prefStyled = grey.Render(mark) + grey.Render("[") + st.Render(whoCell) + grey.Render("]")
	}

	if layout := timePrec.goTimeLayout(); layout != "" {
		ts := rec.T.Format(layout)
		tsStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("[" + ts + "]")
		return prefStyled + " " + tsStyled + " " + rec.Msg
	}
	return prefStyled + " " + rec.Msg
}

func neutralPlaceholders(n int) []string {
	out := make([]string, 0, n)
	msg := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		"— merged log (all processes); child output appears here · ? help —",
	)
	out = append(out, msg)
	for i := 1; i < n; i++ {
		out = append(out, lipgloss.NewStyle().Foreground(lipgloss.Color("236")).Render("·"))
	}
	return out
}

func scrolledPastPlaceholders(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = ""
	}
	out[n-1] = lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		"— scrolled past available lines (scan cap or filter) —",
	)
	return out
}
