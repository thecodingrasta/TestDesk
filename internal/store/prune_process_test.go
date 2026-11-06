package store_test

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/store"
)

func TestPruneProtectsRunOwnedByAnotherProcess(t *testing.T) {
	const runID = "20200101T010101.000000000"
	if os.Getenv("TESTDESK_PRUNE_OWNER_HELPER") == "1" {
		s := store.New(os.Getenv("TESTDESK_PRUNE_OWNER_ROOT"), "")
		lease, err := s.BeginRun(runID)
		if err != nil {
			t.Fatal(err)
		}
		defer lease.Close()
		if err := s.SaveRun(domain.Run{ID: runID, Status: domain.StatusRunning, CreatedAt: time.Now().AddDate(0, 0, -30)}); err != nil {
			t.Fatal(err)
		}
		fmt.Println("ready")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	root := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPruneProtectsRunOwnedByAnotherProcess$")
	cmd.Env = append(os.Environ(), "TESTDESK_PRUNE_OWNER_HELPER=1", "TESTDESK_PRUNE_OWNER_ROOT="+root)
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
	t.Cleanup(func() {
		_ = stdin.Close()
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "ready\n" && line != "ready\r\n" {
			t.Fatalf("helper: %q", line)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("helper timed out")
	}
	s := store.New(root, "")
	count, err := s.Prune(7)
	if err != nil || count != 0 {
		t.Fatalf("live prune: %d %v", count, err)
	}
	if _, err := os.Stat(s.DiagnosticsPath(runID)); err != nil {
		t.Fatalf("live diagnostics removed: %v", err)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	if err := s.ReconcileOrphanRuns(); err != nil {
		t.Fatal(err)
	}
	count, err = s.Prune(7)
	if err != nil || count != 0 {
		t.Fatalf("unknown prune: %d %v", count, err)
	}
	if _, err := os.Stat(s.DiagnosticsPath(runID)); err != nil {
		t.Fatalf("unknown diagnostics removed: %v", err)
	}
}
