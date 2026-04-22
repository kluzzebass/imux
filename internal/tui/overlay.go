package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// padToCellWidth pads or truncates s to exactly w terminal cells wide.
func padToCellWidth(s string, w int) string {
	if w <= 0 {
		return ""
	}
	s = ansi.Truncate(s, w, "")
	for ansi.StringWidth(s) < w {
		s += " "
	}
	return s
}

// compositeLogWithModal draws modal centered on top of the log without erasing
// underlying lines (cells outside the modal rectangle stay visible).
func compositeLogWithModal(base, modal string, width, height int) string {
	baseLines := strings.Split(base, "\n")
	for len(baseLines) < height {
		baseLines = append(baseLines, "")
	}
	if len(baseLines) > height {
		baseLines = baseLines[:height]
	}
	for i := range baseLines {
		baseLines[i] = padToCellWidth(baseLines[i], width)
	}

	overlayLines := strings.Split(modal, "\n")
	ow := 0
	for _, ln := range overlayLines {
		if sw := lipgloss.Width(ln); sw > ow {
			ow = sw
		}
	}
	oh := len(overlayLines)
	if ow == 0 || oh == 0 {
		return strings.Join(baseLines, "\n")
	}
	// Every row must occupy the same cell width as the widest row. Otherwise
	// startX is derived from max width but mergeLineAt used each row's own
	// width, leaving a gap where the log showed through (jumbled with help).
	for i := range overlayLines {
		overlayLines[i] = padToCellWidth(overlayLines[i], ow)
	}

	startY := (height - oh) / 2
	startX := (width - ow) / 2
	if startY < 0 {
		startY = 0
	}
	if startX < 0 {
		startX = 0
	}

	for i := 0; i < oh; i++ {
		y := startY + i
		if y < 0 || y >= height {
			continue
		}
		baseLines[y] = mergeLineAt(baseLines[y], overlayLines[i], startX, width)
	}
	return strings.Join(baseLines, "\n")
}

func mergeLineAt(baseLine, insert string, startCol, totalWidth int) string {
	baseLine = padToCellWidth(baseLine, totalWidth)
	ow := lipgloss.Width(insert)
	if ow == 0 {
		ow = ansi.StringWidth(insert)
	}
	if startCol < 0 {
		startCol = 0
	}
	if startCol > totalWidth {
		return baseLine
	}

	left := ansi.Cut(baseLine, 0, startCol)
	rightStart := startCol + ow
	var right string
	if rightStart >= totalWidth {
		right = ""
	} else {
		right = ansi.Cut(baseLine, rightStart, totalWidth)
	}

	merged := left + insert + right
	return padToCellWidth(ansi.Truncate(merged, totalWidth, ""), totalWidth)
}
