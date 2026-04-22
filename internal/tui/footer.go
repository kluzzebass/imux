package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

const footerSep = " · "

// ToastKind selects footer tint for ephemeral status toasts.
type ToastKind int

const (
	ToastNeutral ToastKind = iota
	ToastOK
	ToastErr
)

// StyleFooterMuted is the default dim footer (help strip, key hints).
func StyleFooterMuted(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(s)
}

// StyleFooterToast tints a one-line status message by outcome.
func StyleFooterToast(s string, k ToastKind) string {
	s = strings.TrimSpace(s)
	switch k {
	case ToastOK:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("114")).Render(s)
	case ToastErr:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Render(s)
	default:
		return StyleFooterMuted(s)
	}
}

// StyleFooterPending is used while awaiting a second keypress to confirm.
func StyleFooterPending(s string) string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("222")).Render(s)
}

// joinFooterImportantTrail renders one status line: parts[0] is most important (left).
// trail is always appended last (right), separated by footerSep when non-empty.
// If the line exceeds maxW, the least important parts (highest index in parts) are
// omitted first; trail is kept until maxW cannot fit it alone (then trail truncates).
func joinFooterImportantTrail(parts []string, trail string, maxW int) string {
	if maxW < 1 {
		return ""
	}
	var p []string
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s != "" {
			p = append(p, s)
		}
	}
	trail = strings.TrimSpace(trail)
	trailW := lipgloss.Width(trail)

	if trail == "" {
		for k := len(p); k >= 1; k-- {
			candidate := strings.Join(p[:k], footerSep)
			if lipgloss.Width(candidate) <= maxW {
				return candidate
			}
		}
		if len(p) > 0 {
			return truncate(p[0], maxW)
		}
		return ""
	}

	if trailW > maxW {
		return truncate(trail, maxW)
	}

	for k := len(p); k >= 0; k-- {
		var body string
		if k > 0 {
			body = strings.Join(p[:k], footerSep)
		}
		var candidate string
		if body == "" {
			candidate = trail
		} else {
			candidate = body + footerSep + trail
		}
		if lipgloss.Width(candidate) <= maxW {
			return candidate
		}
	}
	return truncate(trail, maxW)
}
