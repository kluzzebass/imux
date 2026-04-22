package cli

import "fmt"

// Build metadata (overridden via -ldflags at release link time).
var (
	BuildVersion = "dev"
	BuildCommit  = "none"
	BuildDate    = ""
)

// FormatVersion returns a single line suitable for `imux --version` and Homebrew tests.
func FormatVersion() string {
	v := BuildVersion
	if BuildCommit != "" && BuildCommit != "none" {
		return fmt.Sprintf("%s (%s)", v, BuildCommit)
	}
	return v
}
