package tui

import (
	"runtime"
	"testing"

	"imux/internal/core"
)

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
