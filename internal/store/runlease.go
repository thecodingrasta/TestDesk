package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const runLeaseName = ".owner.lock"
const runLeaseVersion = "testdesk-run-lease-v1\n"

// RunLease is held while a process owns a queued or executing run. The OS
// releases its lock if the process exits unexpectedly.
type RunLease struct{ file *os.File }

func (lease *RunLease) Close() error { return lease.file.Close() }

// ValidateRunID keeps run data confined to one directory on every platform.
func ValidateRunID(runID string) error {
	if runID == "" || runID == "." || runID == ".." || strings.TrimSpace(runID) != runID ||
		strings.ContainsAny(runID, `/\:`) || !filepath.IsLocal(runID) ||
		filepath.Base(runID) != runID {
		return errors.New("invalid run ID")
	}
	return nil
}

// BeginRun claims a new run directory before its diagnostics become visible.
func (s *Store) BeginRun(runID string) (*RunLease, error) {
	if err := s.Ensure(); err != nil {
		return nil, err
	}
	if err := ValidateRunID(runID); err != nil {
		return nil, err
	}
	if err := os.Mkdir(s.RunDir(runID), 0o700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(s.RunDir(runID), runLeaseName), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	locked, err := tryLockRun(file)
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("claim run lease: %w", err)
	}
	if !locked {
		_ = file.Close()
		return nil, errors.New("new run lease unexpectedly locked")
	}
	if _, err := file.WriteString(runLeaseVersion); err != nil {
		_ = file.Close()
		return nil, err
	}
	return &RunLease{file: file}, nil
}

// orphanLease only returns a lease for a known-format, currently unowned run.
// Legacy runs have no trustworthy ownership evidence and are left untouched.
func (s *Store) orphanLease(runID string) (*RunLease, error) {
	file, err := os.OpenFile(filepath.Join(s.RunDir(runID), runLeaseName), os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	locked, err := tryLockRun(file)
	if err != nil || !locked {
		_ = file.Close()
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(len(runLeaseVersion)+1)))
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if string(data) != runLeaseVersion {
		_ = file.Close()
		return nil, nil
	}
	return &RunLease{file: file}, nil
}
