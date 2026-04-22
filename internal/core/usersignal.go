package core

import "context"

// UserSignal is a TUI kill-menu choice (mapped to OS semantics in SendUserSignal).
type UserSignal int

const (
	// UserSignalStopGraceful is SIGTERM + wait (+ SIGKILL after grace) on unix; full Stop on all platforms.
	UserSignalStopGraceful UserSignal = iota
	// UserSignalInterrupt is SIGINT on unix; on Windows maps to force Kill.
	UserSignalInterrupt
	// UserSignalHangup is SIGHUP on unix; on Windows maps to force Kill.
	UserSignalHangup
	// UserSignalForceKill is SIGKILL / hard terminate.
	UserSignalForceKill
	// UserSignalSIGTERMOnce sends one SIGTERM to the group without the supervisor stop/wait/kill sequence.
	UserSignalSIGTERMOnce
	// UserSignalSIGQUIT sends SIGQUIT (often core dump + exit on unix).
	UserSignalSIGQUIT
	// UserSignalSIGUSR1 / SIGUSR2 are application-defined; default is usually to keep running.
	UserSignalSIGUSR1
	UserSignalSIGUSR2
	// UserSignalSIGPIPE default action is often terminate.
	UserSignalSIGPIPE
	// UserSignalSIGWINCH informs the child of terminal size changes; default is usually ignore.
	UserSignalSIGWINCH
	// UserSignalSIGALRM is the alarm clock signal; default is often terminate.
	UserSignalSIGALRM
	// UserSignalSIGABRT is abort(3); default is terminate with core.
	UserSignalSIGABRT
)

// SendUserSignal applies the interactive signal/stop choice for id.
func (s *ExecSupervisor) SendUserSignal(ctx context.Context, id ProcessID, choice UserSignal) error {
	switch choice {
	case UserSignalStopGraceful:
		return s.Stop(ctx, id)
	case UserSignalForceKill:
		return s.Kill(ctx, id)
	default:
		return s.sendPOSIXMenuSignal(ctx, id, choice)
	}
}
