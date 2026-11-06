package store

import (
	"os"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestStoreCatalogAndHistory(t *testing.T) {
	dir := t.TempDir()
	st := New(dir, ".testdesk")
	if err := st.Ensure(); err != nil {
		t.Fatal(err)
	}

	tests := []domain.TestCase{
		{ID: "test-1", Name: "Test One", FullName: "Suite.Test One", ProviderID: "p1"},
	}
	if err := st.SaveCatalog(tests); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[0].ID != "test-1" {
		t.Fatalf("unexpected loaded catalog: %+v", loaded)
	}

	run := domain.Run{
		ID:        "20260101T120000.000000000",
		Status:    domain.StatusPassed,
		CreatedAt: time.Now().UTC(),
	}
	if err := st.AppendHistory(run); err != nil {
		t.Fatal(err)
	}
	recent, err := st.RecentRuns(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].ID != run.ID {
		t.Fatalf("unexpected recent runs: %+v", recent)
	}
}

func TestStorePruneRemovesExpiredRunsAndHistory(t *testing.T) {
	dir := t.TempDir()
	st := New(dir, ".testdesk")
	if err := st.Ensure(); err != nil {
		t.Fatal(err)
	}

	oldRunID := "20200101T120000.000000000"
	newRunID := time.Now().UTC().Format("20060102T150405.000000000")
	oldLease, err := st.BeginRun(oldRunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := oldLease.Close(); err != nil {
		t.Fatal(err)
	}
	newLease, err := st.BeginRun(newRunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := newLease.Close(); err != nil {
		t.Fatal(err)
	}

	// Append logs and history for both runs
	if err := st.AppendLog(oldRunID, "old log"); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendLog(newRunID, "new log"); err != nil {
		t.Fatal(err)
	}

	oldRun := domain.Run{
		ID:        oldRunID,
		Status:    domain.StatusPassed,
		CreatedAt: time.Now().UTC().AddDate(0, 0, -30),
	}
	newRun := domain.Run{
		ID:        newRunID,
		Status:    domain.StatusPassed,
		CreatedAt: time.Now().UTC(),
	}
	if err := st.SaveRun(oldRun); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveRun(newRun); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendHistory(oldRun); err != nil {
		t.Fatal(err)
	}
	if err := st.AppendHistory(newRun); err != nil {
		t.Fatal(err)
	}

	// Verify both exist before prune
	if _, err := os.Stat(st.RunDir(oldRunID)); err != nil {
		t.Fatalf("old run dir should exist before prune: %v", err)
	}
	if _, err := os.Stat(st.RunDir(newRunID)); err != nil {
		t.Fatalf("new run dir should exist before prune: %v", err)
	}

	// Prune with 7 day retention
	pruned, err := st.Prune(7)
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}
	if pruned != 1 {
		t.Fatalf("expected 1 pruned run, got %d", pruned)
	}

	// Verify old run dir is gone, new run dir is kept
	if _, err := os.Stat(st.RunDir(oldRunID)); !os.IsNotExist(err) {
		t.Fatalf("old run dir should be deleted, err: %v", err)
	}
	if _, err := os.Stat(st.RunDir(newRunID)); err != nil {
		t.Fatalf("new run dir should still exist: %v", err)
	}

	// Verify history only has new run
	recent, err := st.RecentRuns(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].ID != newRunID {
		t.Fatalf("history was not pruned: %+v", recent)
	}
}
