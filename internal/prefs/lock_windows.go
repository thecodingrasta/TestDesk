package prefs

import (
	"golang.org/x/sys/windows"
	"os"
)

func lockPreferences(file *os.File) error {
	var overlap windows.Overlapped
	return windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, &overlap)
}
func unlockPreferences(file *os.File) {
	var overlap windows.Overlapped
	_ = windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, &overlap)
}
