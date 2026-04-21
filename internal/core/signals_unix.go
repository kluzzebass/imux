//go:build !windows

package core

import (
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

func applySysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func signalProcessGroup(pid int, sig syscall.Signal) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	return syscall.Kill(-pid, sig)
}

func sendSIGTERMToGroup(pid int) error {
	return signalProcessGroup(pid, syscall.SIGTERM)
}

func killProcessGroup(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("kill: invalid pid %d", pid)
	}
	return signalProcessGroup(pid, syscall.SIGKILL)
}

// hardKill sends SIGKILL to the child's process group (unix).
func hardKill(cmd *exec.Cmd, pid int) error {
	_ = cmd
	return killProcessGroup(pid)
}

func pauseProcessGroup(pid int) error {
	return signalProcessGroup(pid, syscall.SIGSTOP)
}

func continueProcessGroup(pid int) error {
	return signalProcessGroup(pid, syscall.SIGCONT)
}

// procStopWait sends SIGTERM to the group, waits for exit or kills after grace.
func procStopWait(cmd *exec.Cmd, pid int, grace time.Duration, waitDone <-chan struct{}) error {
	if err := sendSIGTERMToGroup(pid); err != nil {
		return err
	}
	t := time.NewTimer(grace)
	defer t.Stop()
	select {
	case <-waitDone:
		return nil
	case <-t.C:
		_ = hardKill(cmd, pid)
	}
	<-waitDone
	return nil
}
