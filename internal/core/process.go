package core

// ProcessID is a stable identifier for a managed process.
type ProcessID string

// RestartPolicy controls restart behavior after process exit.
type RestartPolicy string

const (
	RestartNever     RestartPolicy = "never"
	RestartOnFailure RestartPolicy = "on_failure"
	RestartAlways    RestartPolicy = "always"
)

// ProcessSpec defines an executable unit under supervisor control.
type ProcessSpec struct {
	ID      ProcessID
	Name    string
	Command string
	Args    []string
	Env     map[string]string
	Dir     string

	Restart RestartConfig
}

// RestartConfig defines policy and optional attempt limits.
type RestartConfig struct {
	Policy      RestartPolicy
	MaxRestarts int
}

// ExitReason tells why a process ended.
type ExitReason string

const (
	ExitReasonCompleted ExitReason = "completed"
	ExitReasonFailed    ExitReason = "failed"
	ExitReasonSignaled  ExitReason = "signaled"
	ExitReasonRequested ExitReason = "requested"
)

// ExitResult carries outcome data for restart-policy evaluation.
type ExitResult struct {
	Reason       ExitReason
	ExitCode     int
	RestartCount int
}
