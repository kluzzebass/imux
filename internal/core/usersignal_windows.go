//go:build windows

package core

import (
	"context"
	"fmt"
)

// sendPOSIXMenuSignal maps a subset of menu choices to Windows semantics.
func (s *ExecSupervisor) sendPOSIXMenuSignal(ctx context.Context, id ProcessID, choice UserSignal) error {
	switch choice {
	case UserSignalInterrupt, UserSignalHangup:
		return s.Kill(ctx, id)
	default:
		return fmt.Errorf("send signal: choice %d is not supported on Windows (use graceful stop or force terminate)", choice)
	}
}
