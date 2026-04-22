package tui

import "testing"

func TestParseLogFilter(t *testing.T) {
	t.Parallel()
	p, err := ParseLogFilter("")
	if err != nil || p != "" {
		t.Fatalf("empty: got %q err=%v", p, err)
	}
	p, err = ParseLogFilter("  foo.*  ")
	if err != nil || p != "foo.*" {
		t.Fatalf("bare: got %q err=%v", p, err)
	}
	p, err = ParseLogFilter("re:bar")
	if err != nil || p != "bar" {
		t.Fatalf("re: got %q err=%v", p, err)
	}
	_, err = ParseLogFilter("re:")
	if err == nil {
		t.Fatal("re: empty want error")
	}
	_, err = ParseLogFilter("glob:x")
	if err == nil {
		t.Fatal("glob: want error")
	}
	_, err = ParseLogFilter("[")
	if err == nil {
		t.Fatal("invalid regexp want error")
	}
}
