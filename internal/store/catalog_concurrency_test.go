package store_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/store"
)

func TestCatalogProcessHelper(t *testing.T) {
	if os.Getenv("TESTDESK_CATALOG_HELPER") != "1" {
		return
	}
	root, id := os.Getenv("TESTDESK_CATALOG_ROOT"), os.Getenv("TESTDESK_CATALOG_ID")
	if err := os.WriteFile(filepath.Join(root, "ready-"+id), nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "start")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("start barrier timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := store.New(root, ".testdesk").UpdateCatalogResults([]domain.TestCase{{ID: id, LastStatus: domain.StatusPassed, LastRunID: id}}); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogConcurrentProcessTransactions(t *testing.T) {
	root := t.TempDir()
	const count = 8
	tests := make([]domain.TestCase, count)
	for i := range tests {
		tests[i] = domain.TestCase{ID: fmt.Sprint(i), LastStatus: domain.StatusUnknown}
	}
	if err := store.New(root, ".testdesk").SaveCatalog(tests); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	children := make([]*exec.Cmd, 0, count)
	t.Cleanup(func() {
		for _, cmd := range children {
			if cmd.ProcessState == nil {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
			}
		}
	})
	for i := range tests {
		cmd := exec.Command(exe, "-test.run=^TestCatalogProcessHelper$")
		cmd.Env = append(os.Environ(), "TESTDESK_CATALOG_HELPER=1", "TESTDESK_CATALOG_ROOT="+root, "TESTDESK_CATALOG_ID="+fmt.Sprint(i))
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, cmd)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		ready, err := filepath.Glob(filepath.Join(root, "ready-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(ready) == count {
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
	for _, child := range children {
		if err := child.Wait(); err != nil {
			t.Errorf("helper: %v", err)
		}
	}
	loaded, err := store.New(root, ".testdesk").LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != count {
		t.Fatalf("got %d tests, want %d", len(loaded), count)
	}
	for _, test := range loaded {
		if test.LastStatus != domain.StatusPassed || test.LastRunID != test.ID {
			t.Errorf("lost process update: %+v", test)
		}
	}
}

func TestCatalogConcurrentResultTransactions(t *testing.T) {
	root := t.TempDir()
	const count = 32
	tests := make([]domain.TestCase, count)
	for i := range tests {
		tests[i] = domain.TestCase{ID: fmt.Sprint(i), LastStatus: domain.StatusUnknown}
	}
	if err := store.New(root, ".testdesk").SaveCatalog(tests); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	errors := make(chan error, count)
	for i := range tests {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			_, err := store.New(root, ".testdesk").UpdateCatalogResults([]domain.TestCase{{ID: fmt.Sprint(index), LastStatus: domain.StatusPassed, LastRunID: fmt.Sprintf("run-%d", index)}})
			if err != nil {
				errors <- err
			}
		}(i)
	}
	group.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	loaded, err := store.New(root, ".testdesk").LoadCatalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != count {
		t.Fatalf("got %d tests, want %d", len(loaded), count)
	}
	for _, test := range loaded {
		if test.LastStatus != domain.StatusPassed || test.LastRunID == "" {
			t.Errorf("lost result: %+v", test)
		}
	}
}

func TestCatalogDiscoveryPreservesResultsAndDoesNotResurrectRemovedTests(t *testing.T) {
	s := store.New(t.TempDir(), ".testdesk")
	if err := s.SaveCatalog([]domain.TestCase{{ID: "kept"}, {ID: "removed"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.UpdateCatalogResults([]domain.TestCase{{ID: "kept", LastStatus: domain.StatusPassed, LastRunID: "run"}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReplaceCatalog([]domain.TestCase{{ID: "kept", Name: "rediscovered"}})
	if err != nil || len(got) != 1 || got[0].LastRunID != "run" || got[0].Name != "rediscovered" {
		t.Fatalf("replacement: %+v %v", got, err)
	}
	got, err = s.UpdateCatalogResults([]domain.TestCase{{ID: "removed", LastStatus: domain.StatusFailed, LastRunID: "later"}})
	if err != nil || len(got) != 1 || got[0].ID != "kept" {
		t.Fatalf("removed identity resurrected: %+v %v", got, err)
	}
}
