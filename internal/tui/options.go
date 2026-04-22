package tui

import (
	"fmt"
	"regexp"
	"strings"
)

// BootstrapProc is one shell command to register and start before the TUI first paints.
type BootstrapProc struct {
	ID   string // stable slot id (also the initial display name until edited)
	Line string // full user command line (wrapped with sh -c / cmd /C)
}

// Options configures the TUI session (CLI flags).
type Options struct {
	TeePath   string
	LogFilter string // optional: "re:PAT" or bare PAT (Go regexp); empty = no filter
	Bootstrap []BootstrapProc
}

// ParseLogFilter returns the regexp pattern, or empty for no filter.
// Accepts "re:PAT" or bare PAT (both must compile as Go regex).
// Legacy "glob:…" is rejected.
func ParseLogFilter(s string) (pattern string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	if strings.HasPrefix(s, "glob:") {
		return "", fmt.Errorf("log filter: glob mode was removed; use re:… or a bare regular expression")
	}
	if strings.HasPrefix(s, "re:") {
		p := strings.TrimPrefix(s, "re:")
		if p == "" {
			return "", fmt.Errorf("log filter: empty pattern after re:")
		}
		if _, err := regexp.Compile(p); err != nil {
			return "", fmt.Errorf("log filter regexp: %w", err)
		}
		return p, nil
	}
	if _, err := regexp.Compile(s); err != nil {
		return "", fmt.Errorf("log filter regexp: %w", err)
	}
	return s, nil
}
