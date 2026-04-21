//go:build windows

package core

import (
	"fmt"
	"os/exec"
	"time"
)

func applySysProcAttr(cmd *exec.Cmd) {
	_ = cmd
}

// procStopWait waits for natural exit or kills after grace (no POSIX process groups).
func procStopWait(cmd *exec.Cmd, pid int, grace time.Duration, waitDone <-chan struct{}) error {
	_ = pid
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("stop: process is not running")
	}
	t := time.NewTimer(grace)
	defer t.Stop()
	select {
	case <-waitDone:
		return nil
	case <-t.C:
		if err := hardKill(cmd, 0); err != nil {
			return err
		}
	}
	<-waitDone
	return nil
}

// hardKill terminates the child process.
func hardKill(cmd *exec.Cmd, pid int) error {
	_ = pid
	if cmd == nil || cmd.Process == nil {
		return fmt.Errorf("kill: process is not running")
	}
	return cmd.Process.Kill()
}

func pauseProcessGroup(pid int) error {
	_ = pid
	return fmt.Errorf("pause is not supported on Windows: stop or kill the process instead")
}

func continueProcessGroup(pid int) error {
	_ = pid
	return fmt.Errorf("continue is not supported on Windows: start the process again if it was stopped")
}
