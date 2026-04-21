package core

// ProcessState tracks current lifecycle state for each process.
type ProcessState string

const (
	StatePending  ProcessState = "pending"
	StateStarting ProcessState = "starting"
	StateRunning  ProcessState = "running"
	StatePaused   ProcessState = "paused"
	StateStopping ProcessState = "stopping"
	StateExited   ProcessState = "exited"
	StateFailed   ProcessState = "failed"
)

// Snapshot is a point-in-time view of process states.
type Snapshot struct {
	Processes map[ProcessID]ProcessState
}

// StateStore is the read/write boundary for process lifecycle state.
type StateStore interface {
	Set(ProcessID, ProcessState)
	Get(ProcessID) (ProcessState, bool)
	Snapshot() Snapshot
}

// ApplyLifecycleEvent advances a single process state from a lifecycle event.
func ApplyLifecycleEvent(current ProcessState, eventType EventType) (ProcessState, bool) {
	switch eventType {
	case EventProcessRegistered:
		return StatePending, true
	case EventProcessStarting:
		if current == StatePending || current == StateExited || current == StateFailed {
			return StateStarting, true
		}
	case EventProcessRunning:
		if current == StateStarting {
			return StateRunning, true
		}
	case EventProcessPaused:
		if current == StateRunning {
			return StatePaused, true
		}
	case EventProcessResumed:
		if current == StatePaused {
			return StateRunning, true
		}
	case EventProcessStopping:
		if current == StateRunning || current == StateStarting || current == StatePaused {
			return StateStopping, true
		}
	case EventProcessExited:
		if current == StateStopping || current == StateRunning || current == StateStarting || current == StatePaused {
			return StateExited, true
		}
	case EventProcessFailed:
		if current == StateStarting || current == StateRunning || current == StateStopping || current == StatePaused {
			return StateFailed, true
		}
	}

	return current, false
}
