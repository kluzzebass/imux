package tui

import (
	"testing"

	"imux/internal/core"
)

func TestDisplayNameConflicts(t *testing.T) {
	t.Parallel()
	specs := []core.ProcessSpec{
		{ID: "a", Name: "foo"},
		{ID: "b", Name: "Bar"},
		{ID: "c", Name: ""},
	}
	if _, ok := displayNameConflicts(specs, "a", "foo"); ok {
		t.Fatal("same slot should not conflict with itself")
	}
	if id, ok := displayNameConflicts(specs, "b", "foo"); !ok || id != "a" {
		t.Fatalf("want conflict with a for foo, got id=%q ok=%v", id, ok)
	}
	if id, ok := displayNameConflicts(specs, "a", "bar"); !ok || id != "b" {
		t.Fatalf("want conflict with b for bar, got id=%q ok=%v", id, ok)
	}
	if id, ok := displayNameConflicts(specs, "a", "c"); !ok || id != "c" {
		t.Fatalf("empty name falls back to id: want conflict with c, got id=%q ok=%v", id, ok)
	}
	if _, ok := displayNameConflicts(specs, "", "new"); ok {
		t.Fatal("fresh name should not conflict")
	}
}

func TestResolvedNameFromBuffers(t *testing.T) {
	t.Parallel()
	if got := resolvedNameFromBuffers("  x  ", "sleep 1"); got != "x" {
		t.Fatalf("explicit name: %q", got)
	}
	if got := resolvedNameFromBuffers("", "sleep 1"); got != "sleep" {
		t.Fatalf("from command: %q", got)
	}
	if got := resolvedNameFromBuffers("", ""); got != "proc" {
		t.Fatalf("fallback: %q", got)
	}
}
