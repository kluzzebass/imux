package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestWrapLogLineWithBadgeIndent_continuationIndent(t *testing.T) {
	prefix := "[ps] "
	msg := strings.Repeat("x", 30)
	lines := wrapLogLineWithBadgeIndent(prefix, msg, 14)
	for i, ln := range lines {
		if sw := ansi.StringWidth(ln); sw > 14 {
			t.Fatalf("line %d width %d > 14: %q", i, sw, ln)
		}
	}
	if len(lines) < 2 {
		t.Fatalf("expected wrap into multiple lines, got %d", len(lines))
	}
	pw := ansi.StringWidth(prefix)
	wantIndent := strings.Repeat(" ", pw)
	if !strings.HasPrefix(lines[1], wantIndent) {
		t.Fatalf("line 1 should start with %d spaces, got %q", pw, lines[1])
	}
	if !strings.HasPrefix(lines[0], prefix) {
		t.Fatalf("line 0 should start with prefix, got %q", lines[0])
	}
}

func TestWrapLogLineWithBadgeIndent_emptyMsg(t *testing.T) {
	got := wrapLogLineWithBadgeIndent("P ", "", 10)
	if len(got) != 1 {
		t.Fatalf("len %d", len(got))
	}
}

func TestWrapLogLineWithBadgeIndent_narrowTerminal(t *testing.T) {
	// Prefix alone wider than view: single truncated row.
	longPrefix := strings.Repeat("a", 20)
	got := wrapLogLineWithBadgeIndent(longPrefix, "tail", 8)
	if len(got) != 1 {
		t.Fatalf("want 1 line, got %d", len(got))
	}
	if ansi.StringWidth(got[0]) > 8 {
		t.Fatalf("width %d", ansi.StringWidth(got[0]))
	}
}
