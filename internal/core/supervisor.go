package core

import "context"

// Supervisor orchestrates process lifecycle operations.
type Supervisor interface {
	Register(context.Context, ProcessSpec) error
	Start(context.Context, ProcessID) error
	Stop(context.Context, ProcessID) error
	Kill(context.Context, ProcessID) error
	Pause(context.Context, ProcessID) error
	Continue(context.Context, ProcessID) error
	Restart(context.Context, ProcessID) error
	List(context.Context) ([]ProcessSpec, error)
}

// ShouldRestart decides whether supervisor should schedule a restart.
func ShouldRestart(config RestartConfig, result ExitResult) bool {
	if config.MaxRestarts > 0 && result.RestartCount >= config.MaxRestarts {
		return false
	}

	// User-requested shutdown is terminal, regardless of policy.
	if result.Reason == ExitReasonRequested {
		return false
	}

	switch config.Policy {
	case RestartAlways:
		return true
	case RestartOnFailure:
		return result.Reason == ExitReasonFailed || result.Reason == ExitReasonSignaled
	case RestartNever:
		fallthrough
	default:
		return false
	}
}
