package lifecycle

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func claimOwnerLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	var overlap windows.Overlapped
	err = windows.LockFileEx(windows.Handle(file.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY, 0, 1, 0, &overlap)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
			return nil, errors.New("TestDesk is already running")
		}
		return nil, fmt.Errorf("lock TestDesk desktop owner: %w", err)
	}
	return file, nil
}
