package tui

import (
	"fmt"
	"regexp"
	"strings"
)

// Options configures the TUI session (CLI flags).
type Options struct {
	TeePath   string
	LogFilter string // optional: "glob:PAT" or "re:PAT"; bare string treated as glob
}

// ParseLogFilter returns (isRegex, pattern, err). Empty input means no filter.
func ParseLogFilter(s string) (isRegex bool, pattern string, err error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return false, "", nil
	}
	if strings.HasPrefix(s, "re:") {
		p := strings.TrimPrefix(s, "re:")
		if p == "" {
			return true, "", fmt.Errorf("log filter: empty pattern after re:")
		}
		if _, err := regexp.Compile(p); err != nil {
			return true, "", fmt.Errorf("log filter regexp: %w", err)
		}
		return true, p, nil
	}
	if strings.HasPrefix(s, "glob:") {
		p := strings.TrimPrefix(s, "glob:")
		if p == "" {
			return false, "", fmt.Errorf("log filter: empty pattern after glob:")
		}
		return false, p, nil
	}
	// Default: glob
	return false, s, nil
}
