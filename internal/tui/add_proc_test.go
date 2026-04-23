package tui

import (
	"runtime"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"imux/internal/core"
)

func TestNameFromCommandLine(t *testing.T) {
	t.Parallel()
	if got := nameFromCommandLine("  "); got != "proc" {
		t.Fatalf("whitespace: %q", got)
	}
	if got := nameFromCommandLine("sleep 9"); got != "sleep" {
		t.Fatalf("fields: %q", got)
	}
}

func TestSanitizeDisplayName(t *testing.T) {
	t.Parallel()
	if got := sanitizeDisplayName("  foo\nbar  "); got != "foo bar" {
		t.Fatalf("got %q", got)
	}
}

func TestInnerCommandForEditShWrap(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX sh -c")
	}
	sp := core.ProcessSpec{Command: "sh", Args: []string{"-c", "sleep 1"}}
	if got := innerCommandForEdit(sp); got != "sleep 1" {
		t.Fatalf("got %q", got)
	}
}

func TestInnerCommandForEditNonWrap(t *testing.T) {
	t.Parallel()
	sp := core.ProcessSpec{Command: "sleep", Args: []string{"1"}}
	if got := innerCommandForEdit(sp); got != "sleep 1" {
		t.Fatalf("got %q", got)
	}
}

func TestNameLineTIViewWidth(t *testing.T) {
	t.Parallel()
	const innerW = 40
	ti := newNameLineTI()
	ti.Width = lineFormTextWidth(innerW, ti)
	ti.SetValue("foo")
	// textinput may render the blinking cursor one cell past Width in some versions.
	if w := lipgloss.Width(ti.View()); w > innerW+2 {
		t.Fatalf("line much wider than innerW: %d > %d+2", w, innerW)
	}
}

func TestPadLogNamePlain(t *testing.T) {
	t.Parallel()
	if got := padLogNamePlain("ps2", 6); got != "   ps2" || lipgloss.Width(got) != 6 {
		t.Fatalf("left pad / right align: %q width %d", got, lipgloss.Width(got))
	}
	if got := padLogNamePlain("  ", 4); got != "   ?" || lipgloss.Width(got) != 4 {
		t.Fatalf("empty-ish: %q w=%d", got, lipgloss.Width(got))
	}
}

func TestDockNameColumnWidth(t *testing.T) {
	t.Parallel()
	if w := dockNameColumnWidth([]string{"a", "bcdef"}, 4, 32); w != 5 {
		t.Fatalf("want 5 got %d", w)
	}
	if w := dockNameColumnWidth([]string{"verylongnamefortestinghere"}, 4, 8); w != 8 {
		t.Fatalf("clamp max: got %d", w)
	}
}

func TestWrapModalLinesWidth(t *testing.T) {
	t.Parallel()
	const w = 40
	long := strings.Repeat("abcd ", 30)
	out := wrapModalLines([]string{long}, w, false)
	for _, ln := range out {
		if sw := ansi.StringWidth(ln); sw > w {
			t.Fatalf("line wider than %d: %d %q", w, sw, ln)
		}
	}
}

func TestWrapModalLinesMergeHelpProse(t *testing.T) {
	t.Parallel()
	const w = 120
	lines := []string{
		"Hello world.",
		"Goodbye moon.",
		"",
		"Keys:",
		"  a             b",
	}
	out := wrapModalLines(lines, w, true)
	for _, ln := range out {
		if ln == "" {
			continue
		}
		if sw := ansi.StringWidth(ln); sw > w {
			t.Fatalf("line wider than %d: %d %q", w, sw, ln)
		}
	}
	joined := strings.Join(out, "\n")
	if !strings.Contains(joined, "Hello world. Goodbye moon.") {
		t.Fatalf("expected merged prose on one line at wide width; got:\n%s", joined)
	}
}

func TestExitCodeFromBusMessage(t *testing.T) {
	t.Parallel()
	if n, ok := exitCodeFromBusMessage("exited with code 0"); !ok || n != 0 {
		t.Fatalf("zero: ok=%v n=%d", ok, n)
	}
	if n, ok := exitCodeFromBusMessage("exited with code 17"); !ok || n != 17 {
		t.Fatalf("17: ok=%v n=%d", ok, n)
	}
	if _, ok := exitCodeFromBusMessage("exited with code "); ok {
		t.Fatal("empty after prefix should fail")
	}
	if _, ok := exitCodeFromBusMessage("failed during start: x"); ok {
		t.Fatal("start failure should not parse")
	}
}

func TestFilterPatternTIViewWidth(t *testing.T) {
	t.Parallel()
	const innerW = 40
	ti := newFilterPatternTI()
	ti.Width = lineFormTextWidth(innerW, ti)
	ti.SetValue(strings.Repeat("x", 200))
	if w := lipgloss.Width(ti.View()); w > innerW+2 {
		t.Fatalf("filter row much wider than innerW: %d > %d+2", w, innerW)
	}
}

func TestInnerCommandForEditCmdWrap(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "windows" {
		t.Skip("cmd.exe /C")
	}
	sp := core.ProcessSpec{Command: "cmd.exe", Args: []string{"/C", "echo hi"}}
	if got := innerCommandForEdit(sp); got != "echo hi" {
		t.Fatalf("got %q", got)
	}
}
