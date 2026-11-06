package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

// Store persists catalog, logs and run history under .testdesk by default.
type Store struct {
	root string
	mu   sync.Mutex
}

func New(projectRoot string, workspace string) *Store {
	if workspace == "" {
		workspace = ".testdesk"
	}
	if !filepath.IsAbs(workspace) {
		workspace = filepath.Join(projectRoot, workspace)
	}
	return &Store{root: filepath.Clean(workspace)}
}

func (s *Store) Root() string { return s.root }

func (s *Store) Ensure() error {
	for _, dir := range []string{s.root, filepath.Join(s.root, "runs"), filepath.Join(s.root, "artifacts")} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CatalogPath() string { return filepath.Join(s.root, "catalog.json") }
func (s *Store) HistoryPath() string { return filepath.Join(s.root, "history.jsonl") }
func (s *Store) RunsDir() string     { return filepath.Join(s.root, "runs") }

func (s *Store) CatalogModTime() (time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Stat(s.CatalogPath())
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

func (s *Store) SaveCatalog(tests []domain.TestCase) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.withCatalogLock(func() error { return s.writeCatalogUnlocked(tests) })
}

func (s *Store) LoadCatalog() ([]domain.TestCase, error) {
	tests, _, err := s.LoadCatalogWithModTime()
	return tests, err
}

func (s *Store) LoadCatalogWithModTime() ([]domain.TestCase, time.Time, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var tests []domain.TestCase
	var modified time.Time
	err := s.withCatalogLock(func() error {
		var readErr error
		tests, modified, readErr = s.readCatalogUnlocked()
		return readErr
	})
	return tests, modified, err
}

func (s *Store) readCatalogUnlocked() ([]domain.TestCase, time.Time, error) {
	info, err := os.Stat(s.CatalogPath())
	if errors.Is(err, os.ErrNotExist) {
		return []domain.TestCase{}, time.Time{}, nil
	}
	if err != nil {
		return nil, time.Time{}, err
	}
	data, err := os.ReadFile(s.CatalogPath())
	if err != nil {
		return nil, time.Time{}, err
	}
	var tests []domain.TestCase
	if err := json.Unmarshal(data, &tests); err != nil {
		return nil, time.Time{}, err
	}
	return tests, info.ModTime(), nil
}

func (s *Store) RunDir(runID string) string { return filepath.Join(s.RunsDir(), runID) }

func (s *Store) ArtifactPath(runID string, providerID string, itemID string, extension string) string {
	if extension == "" {
		extension = ".xml"
	}
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	return filepath.Join(s.RunDir(runID), fmt.Sprintf("%s-%s%s", safeFile(providerID), safeFile(itemID), extension))
}

func (s *Store) LogPath(runID string) string { return filepath.Join(s.RunDir(runID), "output.log") }
func (s *Store) DiagnosticsPath(runID string) string {
	return filepath.Join(s.RunDir(runID), "diagnostics.json")
}

func (s *Store) AppendLog(runID string, line string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.RunDir(runID), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(s.LogPath(runID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.WriteString(line + "\n")
	return err
}

func (s *Store) SaveRun(run domain.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.RunDir(run.ID), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.DiagnosticsPath(run.ID), append(data, '\n'), 0o600)
}

func (s *Store) AppendHistory(run domain.Run) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendHistoryLocked(run)
}

func (s *Store) appendHistoryLocked(run domain.Run) error {
	return s.withHistoryLock(func() error { return s.appendHistoryFile(run) })
}

func (s *Store) appendHistoryFile(run domain.Run) error {
	if err := s.Ensure(); err != nil {
		return err
	}
	file, err := os.OpenFile(s.HistoryPath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	data, err := json.Marshal(run)
	if err != nil {
		return err
	}
	if _, err = file.Write(append(data, '\n')); err != nil {
		return err
	}
	return nil
}

func (s *Store) RecentRuns(limit int) ([]domain.Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var runs []domain.Run
	err := s.withHistoryLock(func() error {
		file, err := os.Open(s.HistoryPath())
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer file.Close()
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
		for scanner.Scan() {
			var run domain.Run
			if err := json.Unmarshal(scanner.Bytes(), &run); err == nil {
				if limit > 0 && len(runs) == limit {
					copy(runs, runs[1:])
					runs[len(runs)-1] = run
				} else {
					runs = append(runs, run)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for left, right := 0, len(runs)-1; left < right; left, right = left+1, right-1 {
		runs[left], runs[right] = runs[right], runs[left]
	}
	if runs == nil {
		runs = []domain.Run{}
	}
	return runs, nil
}

func (s *Store) Tail(path string, maxBytes int64) (string, error) {
	if maxBytes <= 0 {
		maxBytes = 32 * 1024
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	start := info.Size() - maxBytes
	if start < 0 {
		start = 0
	}
	if _, err := file.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}
	if start > 0 {
		for len(data) > 0 && (data[0]&0xC0) == 0x80 {
			data = data[1:]
		}
	}
	return strings.ToValidUTF8(string(data), ""), nil
}

// Prune removes run artifacts, diagnostics and output logs older than retentionDays.
// If retentionDays <= 0, no pruning occurs. Returns the number of pruned runs.
func (s *Store) Prune(retentionDays int) (int, error) {
	if retentionDays <= 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	runsDir := s.RunsDir()
	entries, err := os.ReadDir(runsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil
		}
		return 0, err
	}

	prunedCount := 0
	prunedIDs := make(map[string]struct{})
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runID := entry.Name()
		runPath := filepath.Join(runsDir, runID)
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}

		runTime := info.ModTime()
		if t, parseErr := time.Parse("20060102T150405.000000000", runID); parseErr == nil {
			runTime = t
		} else if len(runID) >= 8 {
			if t, parseErr := time.Parse("20060102", runID[:8]); parseErr == nil {
				runTime = t
			}
		}

		if runTime.Before(cutoff) {
			safe, checkErr := s.expiredRunCanBePruned(runID)
			if checkErr != nil {
				return prunedCount, checkErr
			}
			if !safe {
				continue
			}
			if removeErr := os.RemoveAll(runPath); removeErr == nil {
				prunedCount++
				prunedIDs[runID] = struct{}{}
			}
		}
	}

	if err := s.pruneHistoryLocked(prunedIDs); err != nil {
		return prunedCount, err
	}

	return prunedCount, nil
}

// Only a known terminal run with a recognised, currently unheld lease is safe
// to remove. Legacy and unrecognised leases have no reliable ownership proof.
func (s *Store) expiredRunCanBePruned(runID string) (bool, error) {
	leasePath := filepath.Join(s.RunDir(runID), runLeaseName)
	_, statErr := os.Stat(leasePath)
	if errors.Is(statErr, os.ErrNotExist) {
		return false, nil
	}
	if statErr != nil {
		return false, statErr
	}
	lease, err := s.orphanLease(runID)
	if err != nil {
		return false, err
	}
	if lease == nil {
		return false, nil
	}
	if err := lease.Close(); err != nil {
		return false, err
	}
	data, err := os.ReadFile(s.DiagnosticsPath(runID))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	var run domain.Run
	if json.Unmarshal(data, &run) != nil || run.ID != runID {
		return false, nil
	}
	switch run.Status {
	case domain.StatusPassed, domain.StatusFailed, domain.StatusSkipped, domain.StatusCancelled, domain.StatusTimedOut:
		return true, nil
	default:
		return false, nil
	}
}

func (s *Store) pruneHistoryLocked(prunedIDs map[string]struct{}) error {
	if len(prunedIDs) == 0 {
		return nil
	}
	return s.withHistoryLock(func() error { return s.pruneHistoryFile(prunedIDs) })
}

func (s *Store) pruneHistoryFile(prunedIDs map[string]struct{}) error {
	historyPath := s.HistoryPath()
	file, err := os.Open(historyPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer file.Close()

	var kept []string
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var run domain.Run
		line := scanner.Text()
		if err := json.Unmarshal(scanner.Bytes(), &run); err == nil {
			if _, removed := prunedIDs[run.ID]; removed {
				continue
			}
		}
		kept = append(kept, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	_ = file.Close()

	var buf strings.Builder
	for _, line := range kept {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	tmp, err := os.CreateTemp(s.root, ".history-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(buf.String()); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, historyPath)
}

// ReconcileOrphanRuns marks an interrupted run unknown only when its recorded
// ownership lease can be acquired. A free lease does not prove child processes
// have exited or that any particular test outcome occurred.
func (s *Store) ReconcileOrphanRuns() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.RunsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		lease, err := s.orphanLease(entry.Name())
		if err != nil {
			return fmt.Errorf("check run %s ownership: %w", entry.Name(), err)
		}
		if lease == nil {
			continue
		}
		err = s.reconcileLeasedRun(entry.Name())
		closeErr := lease.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (s *Store) reconcileLeasedRun(runID string) error {
	diagPath := s.DiagnosticsPath(runID)
	data, err := os.ReadFile(diagPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var run domain.Run
	if err := json.Unmarshal(data, &run); err != nil {
		return fmt.Errorf("read run %s: %w", runID, err)
	}
	if run.Status != domain.StatusQueued && run.Status != domain.StatusRunning {
		return nil
	}
	for idx := range run.Items {
		switch run.Items[idx].Status {
		case domain.StatusPassed, domain.StatusFailed, domain.StatusSkipped, domain.StatusCancelled:
		default:
			run.Items[idx].Status = domain.StatusUnknown
			run.Items[idx].Error = "execution owner disconnected; child process state and outcome unknown"
		}
	}
	run.Status = domain.StatusUnknown
	updated, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(diagPath, append(updated, '\n'), 0o600); err != nil {
		return err
	}
	return s.appendHistoryLocked(run)
}

func safeFile(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune('-')
		}
	}
	out := strings.Trim(builder.String(), "-")
	if out == "" {
		return "item"
	}
	return out
}
