package store

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// withHistoryLock coordinates history access between independent Store instances.
func (s *Store) withHistoryLock(action func() error) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(s.root, ".history.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	deadline := time.Now().Add(5 * time.Second)
	for {
		locked, err := tryLockRun(file)
		if err != nil {
			return err
		}
		if locked {
			return action()
		}
		if time.Now().After(deadline) {
			return errors.New("timed out waiting for TestDesk history lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
