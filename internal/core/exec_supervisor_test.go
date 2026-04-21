package core

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestExecSupervisorRegisterStartTrueExits(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh -c")
	}
	bus := NewChanEventBus()
	store := NewMapStateStore()
	sup := NewExecSupervisor(bus, store)
	ctx := context.Background()
	id := ProcessID("p1")
	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "one",
		Command: "sh",
		Args:    []string{"-c", "true"},
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st, ok := store.Get(id)
		if ok && st == StateExited {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	st, _ := store.Get(id)
	t.Fatalf("expected exited, got %v", st)
}

func TestExecSupervisorPauseContinueUnix(t *testing.T) {
	t.Parallel()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("SIGSTOP/SIGCONT")
	}
	bus := NewChanEventBus()
	store := NewMapStateStore()
	sup := NewExecSupervisor(bus, store)
	ctx := context.Background()
	id := ProcessID("p1")
	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "sleepy",
		Command: "sleep",
		Args:    []string{"30"},
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := sup.Pause(ctx, id); err != nil {
		t.Fatal(err)
	}
	if st, _ := store.Get(id); st != StatePaused {
		t.Fatalf("after pause want paused, got %q", st)
	}
	if err := sup.Continue(ctx, id); err != nil {
		t.Fatal(err)
	}
	if st, _ := store.Get(id); st != StateRunning {
		t.Fatalf("after continue want running, got %q", st)
	}
	if err := sup.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}
}
