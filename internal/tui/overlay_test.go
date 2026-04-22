package tui

import (
	"strings"
	"testing"
)

func TestCompositeLogWithModal_uniformRowWidth(t *testing.T) {
	t.Parallel()

	width := 40
	height := 5
	base := strings.Repeat("M", width)
	for range height - 1 {
		base += "\n" + strings.Repeat("M", width)
	}
	// Two-line modal: first line much shorter than second (like wrapped help).
	modal := "Hi\n" + strings.Repeat("W", 24)

	out := compositeLogWithModal(base, modal, width, height)
	lines := strings.Split(out, "\n")
	if len(lines) < height {
		t.Fatalf("lines: got %d want at least %d", len(lines), height)
	}
	// First overlay row (startY = (5-2)/2 = 1) is the short "Hi" line padded to
	// the widest row. Without uniform width, log "M" would appear between "Hi"
	// and the right-hand modal edge inside the modal band.
	row := lines[1]
	if !strings.Contains(row, "Hi") {
		t.Fatalf("expected short modal text in row 1: %q", row)
	}
	if strings.Contains(row, "HiM") {
		t.Fatalf("log leaked immediately after short modal text (missing pad): %q", row)
	}
}
