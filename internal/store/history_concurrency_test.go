package store

import (
	"fmt"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRecentRunsSeesOtherStoreUpdates(t *testing.T) {
	root := t.TempDir()
	first := New(root, ".testdesk")
	second := New(root, ".testdesk")
	if _, err := first.RecentRuns(10); err != nil {
		t.Fatal(err)
	}
	if err := second.AppendHistory(domain.Run{ID: "external", Status: domain.StatusPassed, CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	runs, err := first.RecentRuns(10)
	if err != nil || len(runs) != 1 || runs[0].ID != "external" {
		t.Fatalf("history stale: %+v %v", runs, err)
	}
}

func TestConcurrentHistoryProcessHelper(t *testing.T) {
	if os.Getenv("TESTDESK_HISTORY_HELPER") != "1" {
		return
	}
	root := os.Getenv("TESTDESK_HISTORY_ROOT")
	id := os.Getenv("TESTDESK_HISTORY_ID")
	marker := filepath.Join(root, "ready-"+id)
	if err := os.WriteFile(marker, nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "start")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("barrier timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	s := New(root, ".testdesk")
	for i := 0; i < 20; i++ {
		if err := s.AppendHistory(domain.Run{ID: fmt.Sprintf("%s-%d", id, i), Status: domain.StatusPassed, CreatedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
}
func TestConcurrentHistoryPruneAndProcessAppends(t *testing.T) {
	root := t.TempDir()
	s := New(root, ".testdesk")
	oldID := "20200101T010000.000000000"
	old := domain.Run{ID: oldID, Status: domain.StatusPassed, CreatedAt: time.Now().UTC().AddDate(0, 0, -30)}
	lease, err := s.BeginRun(oldID)
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveRun(old); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendHistory(old); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const n = 8
	children := make([]*exec.Cmd, 0, n)
	for i := 0; i < n; i++ {
		cmd := exec.Command(exe, "-test.run=^TestConcurrentHistoryProcessHelper$")
		cmd.Env = append(os.Environ(), "TESTDESK_HISTORY_HELPER=1", "TESTDESK_HISTORY_ROOT="+root, "TESTDESK_HISTORY_ID="+strconv.Itoa(i))
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, cmd)
	}
	t.Cleanup(func() {
		for _, cmd := range children {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for {
		ready, err := filepath.Glob(filepath.Join(root, "ready-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(ready) == n {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helpers did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(root, "start"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Prune(7); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range children {
		if err := cmd.Wait(); err != nil {
			t.Errorf("history writer: %v", err)
		}
	}
	runs, err := s.RecentRuns(0)
	if err != nil || len(runs) != n*20 {
		t.Fatalf("history entries=%d want=%d err=%v", len(runs), n*20, err)
	}
}
