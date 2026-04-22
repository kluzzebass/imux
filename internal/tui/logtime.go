package tui

// logTimePrecision selects how much wall-clock time to show after [stream|name].
type logTimePrecision byte

const (
	logTimeOff logTimePrecision = iota
	logTimeSec
	logTimeMSec
	logTimeUSec
)

const numLogTimePrecisions = 4

func (p logTimePrecision) next() logTimePrecision {
	return logTimePrecision((byte(p) + 1) % numLogTimePrecisions)
}

func (p logTimePrecision) prev() logTimePrecision {
	return logTimePrecision((byte(p) + numLogTimePrecisions - 1) % numLogTimePrecisions)
}

// goTimeLayout returns the Go time.Format layout, or empty when off.
func (p logTimePrecision) goTimeLayout() string {
	switch p {
	case logTimeSec:
		return "15:04:05"
	case logTimeMSec:
		return "15:04:05.000"
	case logTimeUSec:
		return "15:04:05.000000"
	default:
		return ""
	}
}

// statusLabel is a short token for the status line (off, s, ms, us).
func (p logTimePrecision) statusLabel() string {
	switch p {
	case logTimeOff:
		return "off"
	case logTimeSec:
		return "s"
	case logTimeMSec:
		return "ms"
	case logTimeUSec:
		return "us"
	default:
		return "off"
	}
}
