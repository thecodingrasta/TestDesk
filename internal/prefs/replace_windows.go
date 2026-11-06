//go:build windows

package prefs

import (
	"errors"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// Go opens files without FILE_SHARE_DELETE on Windows. While one goroutine or
// process replaces preferences.json, another read of the same path can fail
// with a sharing violation, and a replace can fail while a read is open. Both
// windows are short, so a bounded retry keeps the atomic replace reliable.
const retryWindow = 2 * time.Second

func replaceFile(from, to string) error {
	return retry(func() error { return os.Rename(from, to) })
}

func readFile(path string) ([]byte, error) {
	var data []byte
	err := retry(func() error {
		var readErr error
		data, readErr = os.ReadFile(path)
		return readErr
	})
	return data, err
}

func retry(action func() error) error {
	deadline := time.Now().Add(retryWindow)
	delay := time.Millisecond
	for {
		err := action()
		if err == nil || !retryable(err) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(delay)
		if delay < 50*time.Millisecond {
			delay *= 2
		}
	}
}

func retryable(err error) bool {
	return errors.Is(err, windows.ERROR_ACCESS_DENIED) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
