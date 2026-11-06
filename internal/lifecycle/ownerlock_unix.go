//go:build !windows

package lifecycle

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

func claimOwnerLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	if err != nil {
		_ = file.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, errors.New("TestDesk is already running")
		}
		return nil, fmt.Errorf("lock TestDesk desktop owner: %w", err)
	}
	return file, nil
}
