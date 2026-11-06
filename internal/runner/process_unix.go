//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func terminateProcessTree(process *os.Process) error {
	if process == nil || process.Pid <= 1 {
		return nil
	}
	pgid := process.Pid
	if g, err := syscall.Getpgid(process.Pid); err == nil && g > 1 {
		pgid = g
	}
	if pgid > 1 {
		_ = syscall.Kill(-pgid, syscall.SIGTERM)
		time.Sleep(50 * time.Millisecond)
		_ = syscall.Kill(-pgid, syscall.SIGKILL)
	}
	return process.Kill()
}
