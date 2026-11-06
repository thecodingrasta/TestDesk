package store_test

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/store"
)

func TestRunOwnershipAcrossProcesses(t *testing.T) {
	if os.Getenv("TESTDESK_LEASE_HELPER") == "1" {
		s := store.New(os.Getenv("TESTDESK_LEASE_ROOT"), "")
		lease, err := s.BeginRun("owned")
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()
		if err := s.SaveRun(domain.Run{ID: "owned", Status: domain.StatusRunning, Total: 1, Items: []domain.RunItem{{Status: domain.StatusRunning}}}); err != nil {
			t.Fatal(err)
		}
		fmt.Println("ready")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunOwnershipAcrossProcesses$")
	cmd.Env = append(os.Environ(), "TESTDESK_LEASE_HELPER=1", "TESTDESK_LEASE_ROOT="+root)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		_ = stdin.Close()
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	ready := make(chan string, 1)
	go func() {
		line, _ := bufio.NewReader(stdout).ReadString('\n')
		ready <- line
	}()
	select {
	case line := <-ready:
		if line != "ready\n" && line != "ready\r\n" {
			t.Fatalf("helper failed to start: %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("helper did not acquire run lease")
	}
	s := store.New(root, "")
	if err := s.SaveRun(domain.Run{ID: "legacy", Status: domain.StatusRunning}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileOrphanRuns(); err != nil {
		t.Fatal(err)
	}
	assertRunStatus(t, s, "owned", domain.StatusRunning)
	assertRunStatus(t, s, "legacy", domain.StatusRunning)
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	waited = true
	if err := s.ReconcileOrphanRuns(); err != nil {
		t.Fatal(err)
	}
	run := assertRunStatus(t, s, "owned", domain.StatusUnknown)
	if !run.FinishedAt.IsZero() || len(run.Items) != 1 || run.Items[0].Status != domain.StatusUnknown || run.Items[0].Error == "" {
		t.Fatalf("interrupted run claims a known outcome: %+v", run)
	}
	if err := s.ReconcileOrphanRuns(); err != nil {
		t.Fatal(err)
	}
	history, err := s.RecentRuns(10)
	if err != nil || len(history) != 1 || history[0].ID != "owned" {
		t.Fatalf("history = %+v, %v", history, err)
	}
}

func assertRunStatus(t *testing.T, s *store.Store, id string, want domain.Status) domain.Run {
	t.Helper()
	data, err := os.ReadFile(s.DiagnosticsPath(id))
	if err != nil {
		t.Fatal(err)
	}
	var run domain.Run
	if err := json.Unmarshal(data, &run); err != nil {
		t.Fatal(err)
	}
	if run.Status != want {
		t.Fatalf("run %s status = %s, want %s", id, run.Status, want)
	}
	return run
}
