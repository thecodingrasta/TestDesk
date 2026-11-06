package prefs

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestPrefsConcurrentProcessHelper(t *testing.T) {
	if os.Getenv("TESTDESK_PREFS_HELPER") != "1" {
		return
	}
	dir := os.Getenv("TESTDESK_PREFS_FIXTURE")
	id := os.Getenv("TESTDESK_PREFS_ID")
	if err := os.WriteFile(filepath.Join(dir, "ready-"+id), nil, 0600); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "start")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("barrier timed out")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := UpsertProject(filepath.Join(dir, "project-"+id), "project-"+id); err != nil {
		t.Fatal(err)
	}
}
func TestConcurrentProcessUpsertsPreserveProjects(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TESTDESK_CONFIG_HOME", filepath.Join(dir, "config"))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	const count = 12
	children := make([]*exec.Cmd, 0, count)
	for i := 0; i < count; i++ {
		cmd := exec.Command(exe, "-test.run=^TestPrefsConcurrentProcessHelper$")
		cmd.Env = append(os.Environ(), "TESTDESK_PREFS_HELPER=1", "TESTDESK_PREFS_FIXTURE="+dir, "TESTDESK_PREFS_ID="+strconv.Itoa(i))
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
		ready, err := filepath.Glob(filepath.Join(dir, "ready-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(ready) == count {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("only %d helpers ready", len(ready))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(filepath.Join(dir, "start"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range children {
		if err := cmd.Wait(); err != nil {
			t.Errorf("helper %d: %v", i, err)
		}
	}
	result, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Projects) != count {
		t.Fatalf("lost project updates: got %d, want %d (%s)", len(result.Projects), count, fmt.Sprint(result.Projects))
	}
}

func TestPreferencesLockProcessHelper(t *testing.T) {
	if os.Getenv("TESTDESK_PREFS_LOCK_HELPER") != "1" {
		return
	}
	marker := os.Getenv("TESTDESK_PREFS_LOCK_MARKER")
	err := withPreferencesLock(func() error {
		if err := os.WriteFile(marker, nil, 0600); err != nil {
			return err
		}
		time.Sleep(2 * time.Minute)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
func TestPreferencesLockReleasedWhenOwnerDies(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("TESTDESK_CONFIG_HOME", filepath.Join(dir, "config"))
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(dir, "locked")
	cmd := exec.Command(exe, "-test.run=^TestPreferencesLockProcessHelper$")
	cmd.Env = append(os.Environ(), "TESTDESK_PREFS_LOCK_HELPER=1", "TESTDESK_PREFS_LOCK_MARKER="+marker)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not acquire lock")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	completed := make(chan error, 1)
	go func() { _, err := UpsertProject(filepath.Join(dir, "after"), "after"); completed <- err }()
	select {
	case err := <-completed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("lock remained held after owner death")
	}
}
