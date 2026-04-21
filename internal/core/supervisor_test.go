package core

import "testing"

func TestShouldRestart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		config RestartConfig
		result ExitResult
		want   bool
	}{
		{
			name:   "never policy never restarts",
			config: RestartConfig{Policy: RestartNever},
			result: ExitResult{Reason: ExitReasonFailed},
			want:   false,
		},
		{
			name:   "always policy restarts on normal completion",
			config: RestartConfig{Policy: RestartAlways},
			result: ExitResult{Reason: ExitReasonCompleted},
			want:   true,
		},
		{
			name:   "always policy does not restart requested shutdown",
			config: RestartConfig{Policy: RestartAlways},
			result: ExitResult{Reason: ExitReasonRequested},
			want:   false,
		},
		{
			name:   "on-failure policy restarts failure",
			config: RestartConfig{Policy: RestartOnFailure},
			result: ExitResult{Reason: ExitReasonFailed},
			want:   true,
		},
		{
			name:   "on-failure policy does not restart success",
			config: RestartConfig{Policy: RestartOnFailure},
			result: ExitResult{Reason: ExitReasonCompleted},
			want:   false,
		},
		{
			name:   "restart limit reached",
			config: RestartConfig{Policy: RestartAlways, MaxRestarts: 2},
			result: ExitResult{Reason: ExitReasonFailed, RestartCount: 2},
			want:   false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ShouldRestart(tc.config, tc.result)
			if got != tc.want {
				t.Fatalf("ShouldRestart() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestApplyLifecycleEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		current ProcessState
		event   EventType
		want    ProcessState
		ok      bool
	}{
		{
			name:    "register sets pending",
			current: StatePending,
			event:   EventProcessRegistered,
			want:    StatePending,
			ok:      true,
		},
		{
			name:    "pending to starting",
			current: StatePending,
			event:   EventProcessStarting,
			want:    StateStarting,
			ok:      true,
		},
		{
			name:    "starting to running",
			current: StateStarting,
			event:   EventProcessRunning,
			want:    StateRunning,
			ok:      true,
		},
		{
			name:    "running to stopping",
			current: StateRunning,
			event:   EventProcessStopping,
			want:    StateStopping,
			ok:      true,
		},
		{
			name:    "stopping to exited",
			current: StateStopping,
			event:   EventProcessExited,
			want:    StateExited,
			ok:      true,
		},
		{
			name:    "pending cannot exit directly",
			current: StatePending,
			event:   EventProcessExited,
			want:    StatePending,
			ok:      false,
		},
		{
			name:    "running to paused",
			current: StateRunning,
			event:   EventProcessPaused,
			want:    StatePaused,
			ok:      true,
		},
		{
			name:    "paused to running",
			current: StatePaused,
			event:   EventProcessResumed,
			want:    StateRunning,
			ok:      true,
		},
		{
			name:    "paused to stopping",
			current: StatePaused,
			event:   EventProcessStopping,
			want:    StateStopping,
			ok:      true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := ApplyLifecycleEvent(tc.current, tc.event)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("ApplyLifecycleEvent() = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}
