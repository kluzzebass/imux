package cli

import (
	"fmt"
	"strings"
)

// DuplicateSlotIDs reports whether names contains the same slot id more than once.
func DuplicateSlotIDs(names []string) error {
	seen := make(map[string]int, len(names))
	for i, n := range names {
		if j, ok := seen[n]; ok {
			return fmt.Errorf("duplicate slot id %q (entries %d and %d must differ)", n, j+1, i+1)
		}
		seen[n] = i
	}
	return nil
}

// SplitCSV splits a comma-separated list into non-empty trimmed parts.
func SplitCSV(input string) []string {
	parts := strings.Split(input, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
