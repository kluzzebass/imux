package tui

import "testing"

func TestTrimLastRune(t *testing.T) {
	t.Parallel()
	if got := trimLastRune(""); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got := trimLastRune("a"); got != "" {
		t.Fatalf("one ascii: %q", got)
	}
	if got := trimLastRune("ab"); got != "a" {
		t.Fatalf("ascii: %q", got)
	}
	if got := trimLastRune("é"); got != "" {
		t.Fatalf("one rune: %q", got)
	}
	if got := trimLastRune("aé"); got != "a" {
		t.Fatalf("ascii+rune: %q", got)
	}
}

func TestNameFromCommandLine(t *testing.T) {
	t.Parallel()
	if got := nameFromCommandLine("  "); got != "proc" {
		t.Fatalf("whitespace: %q", got)
	}
	if got := nameFromCommandLine("sleep 9"); got != "sleep" {
		t.Fatalf("fields: %q", got)
	}
}
