//go:build windows

package runner

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"
)

const createNoWindow = 0x08000000

func configureCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

func terminateProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(process.Pid))
	configureCommand(kill)
	_ = kill.Run()
	return process.Kill()
}
