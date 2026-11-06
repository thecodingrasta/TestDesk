package store

import (
	"os"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestPrunePreservesActiveAndUnknownRuns(t *testing.T) {
	s := New(t.TempDir(), ".testdesk")
	if err := s.Ensure(); err != nil {
		t.Fatal(err)
	}
	old := time.Now().UTC().AddDate(0, 0, -30)
	active := "20200101T010000.000000000"
	legacy := "20200101T020000.000000000"
	unknown := "20200101T030000.000000000"
	completed := "20200101T040000.000000000"
	finishing := "20200101T050000.000000000"
	legacyDone := "20200101T060000.000000000"
	lease, err := s.BeginRun(active)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := s.SaveRun(domain.Run{ID: active, Status: domain.StatusRunning, CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(domain.Run{ID: legacy, Status: domain.StatusRunning, CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	orphan, err := s.BeginRun(unknown)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(domain.Run{ID: unknown, Status: domain.StatusUnknown, CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := orphan.Close(); err != nil {
		t.Fatal(err)
	}
	completedLease, err := s.BeginRun(completed)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(domain.Run{ID: completed, Status: domain.StatusPassed, CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	if err := completedLease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(domain.Run{ID: legacyDone, Status: domain.StatusPassed, CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	finishingLease, err := s.BeginRun(finishing)
	if err != nil {
		t.Fatal(err)
	}
	defer finishingLease.Close()
	if err := s.SaveRun(domain.Run{ID: finishing, Status: domain.StatusPassed, CreatedAt: old}); err != nil {
		t.Fatal(err)
	}
	for _, run := range []domain.Run{{ID: active, Status: domain.StatusRunning, CreatedAt: old}, {ID: unknown, Status: domain.StatusUnknown, CreatedAt: old}, {ID: completed, Status: domain.StatusPassed, CreatedAt: old}} {
		if err := s.AppendHistory(run); err != nil {
			t.Fatal(err)
		}
	}
	n, err := s.Prune(7)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d runs; want only completed", n)
	}
	for _, id := range []string{active, legacy, unknown, finishing, legacyDone} {
		if _, err := os.Stat(s.DiagnosticsPath(id)); err != nil {
			t.Errorf("protected run %s lost diagnostics: %v", id, err)
		}
	}
	if _, err := os.Stat(s.RunDir(completed)); !os.IsNotExist(err) {
		t.Errorf("completed run not pruned: %v", err)
	}
	history, err := s.RecentRuns(10)
	if err != nil || len(history) != 2 {
		t.Fatalf("protected history: %+v, %v", history, err)
	}
}
