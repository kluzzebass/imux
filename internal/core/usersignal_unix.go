//go:build !windows

package core

import (
	"context"
	"fmt"
	"syscall"
)

// sendPOSIXMenuSignal delivers TUI-selected POSIX signals to the child's process group.
func (s *ExecSupervisor) sendPOSIXMenuSignal(_ context.Context, id ProcessID, choice UserSignal) error {
	var sys syscall.Signal
	var lifecycle bool // transition to stopping before delivery (typical default: terminate)
	switch choice {
	case UserSignalInterrupt:
		sys, lifecycle = syscall.SIGINT, true
	case UserSignalHangup:
		sys, lifecycle = syscall.SIGHUP, true
	case UserSignalSIGTERMOnce:
		sys, lifecycle = syscall.SIGTERM, true
	case UserSignalSIGQUIT:
		sys, lifecycle = syscall.SIGQUIT, true
	case UserSignalSIGPIPE:
		sys, lifecycle = syscall.SIGPIPE, true
	case UserSignalSIGALRM:
		sys, lifecycle = syscall.SIGALRM, true
	case UserSignalSIGABRT:
		sys, lifecycle = syscall.SIGABRT, true
	case UserSignalSIGUSR1:
		sys, lifecycle = syscall.SIGUSR1, false
	case UserSignalSIGUSR2:
		sys, lifecycle = syscall.SIGUSR2, false
	case UserSignalSIGWINCH:
		sys, lifecycle = syscall.SIGWINCH, false
	default:
		return fmt.Errorf("send signal: unsupported choice %d", choice)
	}

	cmd, pid, cur, _, err := s.procCmdStateAndProc(id)
	if err != nil {
		return err
	}
	if cmd == nil || pid == 0 {
		return fmt.Errorf("send signal: process %q is not running", id)
	}
	if cur == StatePaused {
		_ = continueProcessGroup(pid)
	}

	msg := sys.String()
	if lifecycle {
		s.transition(id, EventProcessStopping, msg)
	}
	if err := signalProcessGroup(pid, sys); err != nil {
		return fmt.Errorf("send signal: %w", err)
	}
	if !lifecycle {
		s.emit(id, EventProcessSignalSent, fmt.Sprintf("sent %s (child may keep running)", msg))
	}
	return nil
}
