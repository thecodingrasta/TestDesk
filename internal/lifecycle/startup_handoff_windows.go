//go:build windows

package lifecycle

import (
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

func attachStartupHandles(cmd *exec.Cmd, read, write *os.File) ([]*os.File, uint64, uint64, error) {
	process, err := windows.GetCurrentProcess()
	if err != nil {
		return nil, 0, 0, err
	}
	var r, w windows.Handle
	if err = windows.DuplicateHandle(process, windows.Handle(read.Fd()), process, &r, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, 0, 0, err
	}
	if err = windows.DuplicateHandle(process, windows.Handle(write.Fd()), process, &w, 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
		windows.CloseHandle(r)
		return nil, 0, 0, err
	}
	read.Close()
	write.Close()
	rf, wf := os.NewFile(uintptr(r), "startup-child-read"), os.NewFile(uintptr(w), "startup-child-write")
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.AdditionalInheritedHandles = append(cmd.SysProcAttr.AdditionalInheritedHandles, syscall.Handle(r), syscall.Handle(w))
	return []*os.File{rf, wf}, uint64(r), uint64(w), nil
}
