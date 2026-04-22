package tui

import (
	"fmt"
	"hash/fnv"
	"path"
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"imux/internal/sessionlog"
)

// maxLogScanLines caps backwards scan cost when rebuilding the match index.
const maxLogScanLines = 100000

var logPalette = []lipgloss.Color{
	"6", "5", "3", "2", "4", "1",
}

func colorStyleForProcess(name, id string) lipgloss.Style {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(id))
	c := logPalette[h.Sum32()%uint32(len(logPalette))]
	return lipgloss.NewStyle().Foreground(c)
}

func flatRecord(rec sessionlog.Record) string {
	tag := string(rec.K)
	if tag == "" {
		tag = "?"
	}
	who := rec.Name
	if who == "" {
		who = rec.ID
	}
	if who == "" {
		who = "?"
	}
	return fmt.Sprintf("[%s|%s] %s", tag, who, rec.Msg)
}

func passesStreamToggles(rec sessionlog.Record, showStdout, showStderr bool) bool {
	switch rec.K {
	case sessionlog.KindStdout:
		return showStdout
	case sessionlog.KindStderr:
		return showStderr
	default:
		return true
	}
}

type compiledFilter struct {
	isRegex bool
	glob    string
	re      *regexp.Regexp
}

func compileFilter(isRegex bool, pattern string) (*compiledFilter, error) {
	if pattern == "" {
		return nil, nil
	}
	if isRegex {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		return &compiledFilter{isRegex: true, re: re}, nil
	}
	return &compiledFilter{isRegex: false, glob: pattern}, nil
}

func (c *compiledFilter) match(flat string) bool {
	if c == nil {
		return true
	}
	if c.isRegex {
		return c.re.MatchString(flat)
	}
	ok, _ := path.Match(c.glob, flat)
	if ok {
		return true
	}
	for _, part := range strings.Fields(flat) {
		if ok, _ := path.Match(c.glob, part); ok {
			return true
		}
	}
	return false
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

// BuildWindowLinesFromIndices renders up to h lines (oldest at [0]) using precomputed
// match indices (newest-first). Only reads at most h lines from disk.
func BuildWindowLinesFromIndices(
	slog *sessionlog.SessionLog,
	indices []int64,
	scrollBack, h int,
	showTime bool,
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
		out = append(out, formatStyledLogLine(rec, showTime))
	}
	for len(out) < h {
		out = append([]string{""}, out...)
	}
	if len(out) > h {
		out = out[len(out)-h:]
	}
	return out, nil
}

func formatStyledLogLine(rec sessionlog.Record, showTime bool) string {
	who := rec.Name
	if who == "" {
		who = string(rec.ID)
	}
	if who == "" {
		who = "?"
	}
	tag := string(rec.K)
	if tag == "" {
		tag = "?"
	}
	prefix := fmt.Sprintf("[%s|%s]", tag, who)
	st := colorStyleForProcess(rec.Name, rec.ID)
	prefStyled := st.Render(prefix)

	var ts string
	if showTime {
		ts = rec.T.Format("15:04:05 ")
	}
	return ts + prefStyled + " " + rec.Msg
}

func neutralPlaceholders(n int) []string {
	out := make([]string, 0, n)
	msg := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		"— merged log (all processes); output appears as children write —",
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
