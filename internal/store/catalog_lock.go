package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func (s *Store) withCatalogLock(action func() error) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(s.root, ".catalog.lock"), os.O_CREATE|os.O_RDWR, 0600)
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
			return errors.New("timed out waiting for TestDesk catalog lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *Store) writeCatalogUnlocked(tests []domain.TestCase) error {
	sort.SliceStable(tests, func(i, j int) bool { return tests[i].ID < tests[j].ID })
	data, err := json.MarshalIndent(tests, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.root, ".catalog-*.tmp")
	if err != nil {
		return err
	}
	path := tmp.Name()
	defer os.Remove(path)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(path, s.CatalogPath())
}

// ReplaceCatalog preserves the latest committed result fields while publishing
// discovery's current set of test identities.
func (s *Store) ReplaceCatalog(tests []domain.TestCase) ([]domain.TestCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.withCatalogLock(func() error {
		current, _, err := s.readCatalogUnlocked()
		if err != nil {
			return err
		}
		byID := make(map[string]domain.TestCase, len(current))
		for _, test := range current {
			byID[test.ID] = test
		}
		for i := range tests {
			if previous, ok := byID[tests[i].ID]; ok {
				copyResultFields(&tests[i], previous)
			}
		}
		return s.writeCatalogUnlocked(tests)
	})
	return tests, err
}

// UpdateCatalogResults changes only the supplied test results in the latest
// catalog; it never recreates identities removed by a concurrent discovery.
func (s *Store) UpdateCatalogResults(changes []domain.TestCase) ([]domain.TestCase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var current []domain.TestCase
	err := s.withCatalogLock(func() error {
		var err error
		current, _, err = s.readCatalogUnlocked()
		if err != nil {
			return err
		}
		byID := make(map[string]domain.TestCase, len(changes))
		for _, test := range changes {
			byID[test.ID] = test
		}
		for i := range current {
			if changed, ok := byID[current[i].ID]; ok {
				copyResultFields(&current[i], changed)
			}
		}
		return s.writeCatalogUnlocked(current)
	})
	return current, err
}

func copyResultFields(target *domain.TestCase, source domain.TestCase) {
	target.LastStatus = source.LastStatus
	target.LastDurationMillis = source.LastDurationMillis
	target.LastRunID = source.LastRunID
	target.LastFailure = source.LastFailure
}
