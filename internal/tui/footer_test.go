package tui

import "testing"

func TestJoinFooterImportantTrail(t *testing.T) {
	parts := []string{"aaa", "bbb", "ccc"}
	trail := "? help"
	if got := joinFooterImportantTrail(parts, trail, 100); got != "aaa · bbb · ccc · ? help" {
		t.Fatalf("wide: %q", got)
	}
	if got := joinFooterImportantTrail(parts, trail, 20); got != "aaa · bbb · ? help" {
		t.Fatalf("medium: %q", got)
	}
	if got := joinFooterImportantTrail(parts, trail, 12); got != "aaa · ? help" {
		t.Fatalf("narrow: %q", got)
	}
	if got := joinFooterImportantTrail(parts, trail, 8); got != "? help" {
		t.Fatalf("trail only: %q", got)
	}
	if got := joinFooterImportantTrail(parts, "", 5); got != "aaa" {
		t.Fatalf("no trail: %q", got)
	}
}
