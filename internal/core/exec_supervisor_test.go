package core

import (
	"context"
	"runtime"
	"strings"
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

func TestExecSupervisorMuxedOutputLines(t *testing.T) {
	t.Parallel()
	bus := NewChanEventBus()
	sub := bus.Subscribe(256)
	store := NewMapStateStore()
	sup := NewExecSupervisor(bus, store)
	ctx := context.Background()
	id := ProcessID("p1")

	shell, args := "sh", []string{"-c", "echo hello; echo err >&2"}
	if runtime.GOOS == "windows" {
		shell, args = "cmd.exe", []string{"/C", "echo hello & echo err 1>&2"}
	}

	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "mux",
		Command: shell,
		Args:    args,
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(ctx, id); err != nil {
		t.Fatal(err)
	}

	type line struct {
		stream, msg string
	}
	var lines []line
	deadline := time.After(5 * time.Second)
	exited := false
	for !exited {
		select {
		case e := <-sub:
			switch e.Type {
			case EventProcessOutput:
				lines = append(lines, line{e.Stream, e.Message})
			case EventProcessExited:
				exited = true
			}
		case <-deadline:
			t.Fatalf("timeout; got %#v", lines)
		}
	}

	has := func(stream, needle string) bool {
		for _, l := range lines {
			if l.stream == stream && strings.Contains(l.msg, needle) {
				return true
			}
		}
		return false
	}
	if !has("o", "hello") {
		t.Fatalf("want stdout line containing hello, got %#v", lines)
	}
	if !has("e", "err") {
		t.Fatalf("want stderr line containing err, got %#v", lines)
	}
}

func TestExecSupervisorCurrentPIDWhileRunning(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
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
		Args:    []string{"5"},
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := sup.CurrentPID(id); ok {
		t.Fatal("did not expect pid before start")
	}
	if err := sup.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	var pid int
	for time.Now().Before(deadline) {
		if p, ok := sup.CurrentPID(id); ok && p > 0 {
			pid = p
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("expected non-zero pid while running")
	}
	_ = sup.Stop(ctx, id)
}

func TestExecSupervisorUnregisterPendingThenRegisterSameID(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sh -c")
	}
	store := NewMapStateStore()
	sup := NewExecSupervisor(NewChanEventBus(), store)
	ctx := context.Background()
	id := ProcessID("slot")
	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "one",
		Command: "sh",
		Args:    []string{"-c", "true"},
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sup.Unregister(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.Get(id); ok {
		t.Fatal("expected process removed from store")
	}
	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "two",
		Command: "sh",
		Args:    []string{"-c", "true"},
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestExecSupervisorUnregisterRejectsRunning(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	sup := NewExecSupervisor(NewChanEventBus(), NewMapStateStore())
	ctx := context.Background()
	id := ProcessID("run")
	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "s",
		Command: "sleep",
		Args:    []string{"30"},
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if err := sup.Unregister(ctx, id); err == nil {
		t.Fatal("expected unregister error while running")
	}
	_ = sup.Stop(ctx, id)
}

func TestExecSupervisorReplaceSpecPending(t *testing.T) {
	t.Parallel()
	sup := NewExecSupervisor(NewChanEventBus(), NewMapStateStore())
	ctx := context.Background()
	id := ProcessID("edit1")
	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "a",
		Command: "sh",
		Args:    []string{"-c", "true"},
		Restart: RestartConfig{Policy: RestartOnFailure, MaxRestarts: 3},
	}); err != nil {
		t.Fatal(err)
	}
	newSpec := ProcessSpec{
		ID:      id,
		Name:    "b",
		Command: "sh",
		Args:    []string{"-c", "echo hi"},
		Restart: RestartConfig{Policy: RestartOnFailure, MaxRestarts: 3},
	}
	if err := sup.ReplaceSpec(ctx, id, newSpec); err != nil {
		t.Fatal(err)
	}
	list, err := sup.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "b" || len(list[0].Args) != 2 || list[0].Args[0] != "-c" || list[0].Args[1] != "echo hi" {
		t.Fatalf("list = %#v", list)
	}
}

func TestExecSupervisorReplaceSpecRejectsRunning(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	sup := NewExecSupervisor(NewChanEventBus(), NewMapStateStore())
	ctx := context.Background()
	id := ProcessID("edit2")
	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "s",
		Command: "sleep",
		Args:    []string{"30"},
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	err := sup.ReplaceSpec(ctx, id, ProcessSpec{
		ID:      id,
		Name:    "x",
		Command: "sh",
		Args:    []string{"-c", "true"},
		Restart: RestartConfig{Policy: RestartNever},
	})
	if err == nil {
		t.Fatal("expected replace spec error while running")
	}
	_ = sup.Stop(ctx, id)
}

func TestExecSupervisorReplaceSpecAfterStopSucceeds(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	sup := NewExecSupervisor(NewChanEventBus(), NewMapStateStore())
	ctx := context.Background()
	id := ProcessID("edit3")
	if err := sup.Register(ctx, ProcessSpec{
		ID:      id,
		Name:    "s",
		Command: "sleep",
		Args:    []string{"30"},
		Restart: RestartConfig{Policy: RestartNever},
	}); err != nil {
		t.Fatal(err)
	}
	if err := sup.Start(ctx, id); err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	newSpec := ProcessSpec{
		ID:      id,
		Name:    "x",
		Command: "sh",
		Args:    []string{"-c", "true"},
		Restart: RestartConfig{Policy: RestartNever},
	}
	if err := sup.ReplaceSpec(ctx, id, newSpec); err == nil {
		t.Fatal("expected replace spec error while running")
	}
	if err := sup.Stop(ctx, id); err != nil {
		t.Fatal(err)
	}
	if err := sup.ReplaceSpec(ctx, id, newSpec); err != nil {
		t.Fatalf("replace after stop: %v", err)
	}
	list, err := sup.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "x" {
		t.Fatalf("list = %#v", list)
	}
}
