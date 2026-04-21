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
)

// Event is emitted by supervisor components and consumed by UI/logging sinks.
type Event struct {
	Type      EventType
	ProcessID ProcessID
	Timestamp time.Time
	Message   string
}

// EventBus is the publish/subscribe boundary between process control and UI.
type EventBus interface {
	Publish(event Event)
	Subscribe(buffer int) <-chan Event
}
