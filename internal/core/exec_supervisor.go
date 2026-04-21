package core

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// ExecSupervisor runs local OS processes with lifecycle events on bus and state in store.
type ExecSupervisor struct {
	mu        sync.Mutex
	bus       EventBus
	store     StateStore
	procs     map[ProcessID]*execProc
	stopGrace time.Duration // optional; SIGTERM to SIGKILL window (defaults to 10s if zero)
}

type execProc struct {
	spec     ProcessSpec
	cmd      *exec.Cmd
	cancel   context.CancelFunc
	pid      int
	sup      *ExecSupervisor
	id       ProcessID
	waitDone chan struct{} // closed when the current cmd.Wait returns
}

// NewExecSupervisor constructs a supervisor backed by bus and store.
func NewExecSupervisor(bus EventBus, store StateStore) *ExecSupervisor {
	return &ExecSupervisor{
		bus:   bus,
		store: store,
		procs: make(map[ProcessID]*execProc),
	}
}

// SetStopGrace sets the stop/kill grace period used after SIGTERM (unix) or before hard kill.
func (s *ExecSupervisor) SetStopGrace(d time.Duration) {
	s.mu.Lock()
	s.stopGrace = d
	s.mu.Unlock()
}

func (s *ExecSupervisor) emit(id ProcessID, typ EventType, msg string) {
	s.bus.Publish(Event{
		Type:      typ,
		ProcessID: id,
		Timestamp: time.Now(),
		Message:   msg,
	})
}

func (s *ExecSupervisor) scanStream(r io.Reader, id ProcessID, name, stream string, wg *sync.WaitGroup) {
	defer wg.Done()
	sc := bufio.NewScanner(r)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		s.bus.Publish(Event{
			Type:        EventProcessOutput,
			ProcessID:   id,
			ProcessName: name,
			Stream:      stream,
			Timestamp:   time.Now(),
			Message:     sc.Text(),
		})
	}
	if err := sc.Err(); err != nil {
		s.emitErr(id, fmt.Sprintf("output read error (%s): %v", stream, err))
	}
}

func (s *ExecSupervisor) emitErr(id ProcessID, msg string) {
	s.emit(id, EventProcessError, msg)
}

// Register records a process specification in pending state.
func (s *ExecSupervisor) Register(_ context.Context, spec ProcessSpec) error {
	if spec.ID == "" {
		return fmt.Errorf("register: ProcessSpec.ID is required")
	}
	if spec.Command == "" {
		return fmt.Errorf("register: ProcessSpec.Command is required for process %q", spec.ID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.procs[spec.ID]; exists {
		return fmt.Errorf("register: process id %q already exists; pick another id or remove the existing process first", spec.ID)
	}

	p := &execProc{
		spec: spec,
		sup:  s,
		id:   spec.ID,
	}
	s.procs[spec.ID] = p
	s.store.Set(spec.ID, StatePending)
	s.emit(spec.ID, EventProcessRegistered, fmt.Sprintf("registered as %q", spec.Name))
	return nil
}

// List returns registered process specs (including not-yet-started).
func (s *ExecSupervisor) List(_ context.Context) ([]ProcessSpec, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]ProcessSpec, 0, len(s.procs))
	for _, p := range s.procs {
		out = append(out, p.spec)
	}
	return out, nil
}

// CurrentPID returns the OS process id for id while the managed process is running
// (including stop/kill in flight). If there is no live OS child, ok is false.
func (s *ExecSupervisor) CurrentPID(id ProcessID) (pid int, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, exists := s.procs[id]
	if !exists || p == nil {
		return 0, false
	}
	if p.pid == 0 {
		return 0, false
	}
	return p.pid, true
}

// Start launches the OS child for id.
func (s *ExecSupervisor) Start(ctx context.Context, id ProcessID) error {
	s.mu.Lock()
	p, ok := s.procs[id]
	if !ok {
		s.mu.Unlock()
		return fmt.Errorf("start: unknown process id %q; register it first", id)
	}
	if p.cmd != nil {
		s.mu.Unlock()
		return fmt.Errorf("start: process %q is already running (pid %d); stop it first or wait for exit", id, p.pid)
	}
	s.mu.Unlock()

	cur, ok := s.store.Get(id)
	if !ok {
		return fmt.Errorf("start: process %q has no state; register it first", id)
	}
	next, allowed := ApplyLifecycleEvent(cur, EventProcessStarting)
	if !allowed {
		return fmt.Errorf("start: process %q is in state %q; only pending/exited/failed processes can be started", id, cur)
	}
	s.store.Set(id, next)
	s.emit(id, EventProcessStarting, "starting")

	runCtx, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(runCtx, p.spec.Command, p.spec.Args...)
	if p.spec.Dir != "" {
		cmd.Dir = p.spec.Dir
	}
	if len(p.spec.Env) > 0 {
		cmd.Env = mergeEnv(os.Environ(), p.spec.Env)
	}
	applySysProcAttr(cmd)

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		cancel()
		s.failStart(id, err)
		return fmt.Errorf("start: stdout pipe: %w", err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		cancel()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		s.failStart(id, err)
		return fmt.Errorf("start: stderr pipe: %w", err)
	}
	cmd.Stdout = stdoutW
	cmd.Stderr = stderrW

	if err := cmd.Start(); err != nil {
		cancel()
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		_ = stderrR.Close()
		_ = stderrW.Close()
		s.failStart(id, err)
		return fmt.Errorf("start: could not launch process %q: %w", id, err)
	}

	waitDone := make(chan struct{})
	s.mu.Lock()
	p.cmd = cmd
	p.cancel = cancel
	p.pid = cmd.Process.Pid
	p.waitDone = waitDone
	s.mu.Unlock()

	s.transition(id, EventProcessRunning, fmt.Sprintf("running pid=%d", p.pid))

	var scanWg sync.WaitGroup
	scanWg.Add(2)
	procName := p.spec.Name
	go p.sup.scanStream(stdoutR, id, procName, "o", &scanWg)
	go p.sup.scanStream(stderrR, id, procName, "e", &scanWg)

	go func() {
		waitErr := cmd.Wait()
		cancel()
		_ = stdoutW.Close()
		_ = stderrW.Close()
		scanWg.Wait()
		_ = stdoutR.Close()
		_ = stderrR.Close()
		p.sup.handleWaitResult(p.id, waitErr)
		close(waitDone)
	}()

	return nil
}

func (s *ExecSupervisor) failStart(id ProcessID, err error) {
	s.emitErr(id, fmt.Sprintf("start failed: %v", err))
	s.store.Set(id, StateFailed)
	s.emit(id, EventProcessFailed, fmt.Sprintf("failed during start: %v", err))
}

func (s *ExecSupervisor) handleWaitResult(id ProcessID, err error) {
	s.mu.Lock()
	if p, ok := s.procs[id]; ok {
		p.cmd = nil
		p.pid = 0
		p.cancel = nil
	}
	s.mu.Unlock()

	cur, ok := s.store.Get(id)
	if !ok {
		return
	}

	if err == nil {
		if _, ok := ApplyLifecycleEvent(cur, EventProcessExited); ok {
			s.store.Set(id, StateExited)
		}
		s.emit(id, EventProcessExited, "exited with code 0")
		return
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if _, ok := ApplyLifecycleEvent(cur, EventProcessFailed); ok {
			s.store.Set(id, StateFailed)
		}
		s.emit(id, EventProcessFailed, fmt.Sprintf("exited with code %d", code))
		return
	}

	if _, ok := ApplyLifecycleEvent(cur, EventProcessFailed); ok {
		s.store.Set(id, StateFailed)
	}
	s.emit(id, EventProcessFailed, fmt.Sprintf("wait error: %v", err))
}

func (s *ExecSupervisor) transition(id ProcessID, ev EventType, msg string) {
	cur, ok := s.store.Get(id)
	if !ok {
		return
	}
	next, allowed := ApplyLifecycleEvent(cur, ev)
	if !allowed {
		s.emitErr(id, fmt.Sprintf("ignored invalid transition to %s while in %s", ev, cur))
		return
	}
	s.store.Set(id, next)
	s.emit(id, ev, msg)
}

// Stop sends SIGTERM (grace) then SIGKILL on unix; on Windows it waits then kills.
func (s *ExecSupervisor) Stop(ctx context.Context, id ProcessID) error {
	_ = ctx
	cmd, pid, cur, p, err := s.procCmdStateAndProc(id)
	if err != nil {
		return err
	}
	if cmd == nil || pid == 0 {
		return fmt.Errorf("stop: process %q is not running; start it before stopping", id)
	}
	if cur == StatePaused {
		if err := continueProcessGroup(pid); err != nil {
			s.emitErr(id, fmt.Sprintf("stop: could not resume before SIGTERM: %v", err))
		}
	}
	s.transition(id, EventProcessStopping, "stopping (SIGTERM then SIGKILL if needed)")

	s.mu.Lock()
	grace := s.stopGrace
	s.mu.Unlock()
	if grace <= 0 {
		grace = 10 * time.Second
	}
	waitCh := p.waitDone
	if waitCh == nil {
		return fmt.Errorf("stop: internal error: missing wait channel for %q", id)
	}
	if err := procStopWait(cmd, pid, grace, waitCh); err != nil && !errors.Is(err, context.Canceled) {
		s.emitErr(id, fmt.Sprintf("stop: %v", err))
	}
	return nil
}

// Kill force-terminates the child process.
func (s *ExecSupervisor) Kill(ctx context.Context, id ProcessID) error {
	_ = ctx
	cmd, pid, cur, p, err := s.procCmdStateAndProc(id)
	if err != nil {
		return err
	}
	if cmd == nil || pid == 0 {
		return fmt.Errorf("kill: process %q is not running; nothing to kill", id)
	}
	if cur == StatePaused {
		_ = continueProcessGroup(pid)
	}
	s.transition(id, EventProcessStopping, "killing (SIGKILL)")
	if err := hardKill(cmd, pid); err != nil {
		s.emitErr(id, fmt.Sprintf("kill: %v", err))
		return fmt.Errorf("kill: %w", err)
	}
	if p.waitDone != nil {
		<-p.waitDone
	}
	return nil
}

// Pause suspends the child process group (SIGSTOP on unix).
func (s *ExecSupervisor) Pause(ctx context.Context, id ProcessID) error {
	_ = ctx
	_, pid, cur, _, err := s.procCmdStateAndProc(id)
	if err != nil {
		return err
	}
	if pid == 0 {
		return fmt.Errorf("pause: process %q is not running", id)
	}
	if cur != StateRunning {
		return fmt.Errorf("pause: process %q is in state %q; only running processes can be paused", id, cur)
	}
	if err := pauseProcessGroup(pid); err != nil {
		s.emitErr(id, fmt.Sprintf("pause: %v", err))
		return fmt.Errorf("pause: %w", err)
	}
	s.transition(id, EventProcessPaused, "paused (SIGSTOP)")
	return nil
}

// Continue resumes a paused process (SIGCONT on unix).
func (s *ExecSupervisor) Continue(ctx context.Context, id ProcessID) error {
	_ = ctx
	_, pid, cur, _, err := s.procCmdStateAndProc(id)
	if err != nil {
		return err
	}
	if pid == 0 {
		return fmt.Errorf("continue: process %q is not running", id)
	}
	if cur != StatePaused {
		return fmt.Errorf("continue: process %q is in state %q; only paused processes can be continued", id, cur)
	}
	if err := continueProcessGroup(pid); err != nil {
		s.emitErr(id, fmt.Sprintf("continue: %v", err))
		return fmt.Errorf("continue: %w", err)
	}
	s.transition(id, EventProcessResumed, "resumed (SIGCONT)")
	return nil
}

// Restart stops a running child (if any) then starts it again.
func (s *ExecSupervisor) Restart(ctx context.Context, id ProcessID) error {
	cmd, _, _, p, err := s.procCmdStateAndProc(id)
	if err != nil {
		return err
	}
	if cmd != nil && p.pid != 0 {
		if err := s.Stop(ctx, id); err != nil {
			return fmt.Errorf("restart: stop failed: %w", err)
		}
		if p.waitDone != nil {
			select {
			case <-p.waitDone:
			case <-time.After(30 * time.Second):
				return fmt.Errorf("restart: timed out waiting for process %q to exit after stop", id)
			}
		}
	}
	return s.Start(ctx, id)
}

func (s *ExecSupervisor) procCmdStateAndProc(id ProcessID) (*exec.Cmd, int, ProcessState, *execProc, error) {
	s.mu.Lock()
	p, ok := s.procs[id]
	if !ok {
		s.mu.Unlock()
		return nil, 0, "", nil, fmt.Errorf("unknown process id %q", id)
	}
	cmd := p.cmd
	pid := p.pid
	s.mu.Unlock()
	st, ok := s.store.Get(id)
	if !ok {
		return nil, 0, "", nil, fmt.Errorf("process %q has no recorded state", id)
	}
	return cmd, pid, st, p, nil
}

func mergeEnv(base []string, extra map[string]string) []string {
	out := append([]string(nil), base...)
	for k, v := range extra {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	return out
}
