package core

import "time"

// EventType describes state-significant supervisor events.
type EventType string

const (
	EventProcessRegistered EventType = "process_registered"
	EventProcessStarting   EventType = "process_starting"
	EventProcessRunning    EventType = "process_running"
	EventProcessStopping   EventType = "process_stopping"
	EventProcessExited     EventType = "process_exited"
	EventProcessFailed     EventType = "process_failed"
	EventProcessPaused     EventType = "process_paused"
	EventProcessResumed    EventType = "process_resumed"
	// EventProcessError reports an operator-visible problem (illegal transition,
	// unsupported platform operation, etc.) without implying a crashed child.
	EventProcessError EventType = "process_error"
	// EventProcessOutput is one logical line from a child stream (stdout/stderr).
	EventProcessOutput EventType = "process_output"
	// EventProcessRemoved means the process slot was dropped from the supervisor.
	EventProcessRemoved EventType = "process_removed"
	// EventProcessSpecUpdated means Command/Args/Name/etc. changed without a lifecycle transition.
	EventProcessSpecUpdated EventType = "process_spec_updated"
	// EventProcessSignalSent is a non-lifecycle POSIX signal delivered to the group (e.g. SIGUSR1).
	// The process is usually left running; store state is unchanged.
	EventProcessSignalSent EventType = "process_signal_sent"
)

// Event is emitted by supervisor components and consumed by UI/logging sinks.
type Event struct {
	Type        EventType
	ProcessID   ProcessID
	ProcessName string // display name when known (e.g. for process_output lines)
	Stream      string // "o" stdout, "e" stderr; empty for non-stream events
	Timestamp   time.Time
	Message     string
}

// EventBus is the publish/subscribe boundary between process control and UI.
type EventBus interface {
	Publish(event Event)
	Subscribe(buffer int) <-chan Event
}
