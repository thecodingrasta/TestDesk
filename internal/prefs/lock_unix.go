//go:build !windows

package prefs

import (
	"golang.org/x/sys/unix"
	"os"
)

func lockPreferences(file *os.File) error { return unix.Flock(int(file.Fd()), unix.LOCK_EX) }
func unlockPreferences(file *os.File)     { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }
