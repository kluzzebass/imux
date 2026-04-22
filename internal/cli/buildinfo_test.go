package cli

import "testing"

func TestFormatVersion(t *testing.T) {
	t.Parallel()
	t.Cleanup(func() {
		BuildVersion = "dev"
		BuildCommit = "none"
		BuildDate = ""
	})
	BuildVersion = "v0.0.1"
	BuildCommit = "abc"
	if got := FormatVersion(); got != "v0.0.1 (abc)" {
		t.Fatalf("with commit: got %q", got)
	}
	BuildCommit = "none"
	if got := FormatVersion(); got != "v0.0.1" {
		t.Fatalf("no commit: got %q", got)
	}
}
