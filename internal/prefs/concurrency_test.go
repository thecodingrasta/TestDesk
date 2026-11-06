package prefs

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentPreferencesReadersAndWriters(t *testing.T) {
	root := t.TempDir()
	t.Setenv("TESTDESK_CONFIG_HOME", filepath.Join(root, "config"))
	const count = 24
	start := make(chan struct{})
	errs := make(chan error, count*2)
	var wait sync.WaitGroup
	for i := 0; i < count; i++ {
		wait.Add(2)
		go func(i int) {
			defer wait.Done()
			<-start
			_, err := UpsertProject(filepath.Join(root, fmt.Sprintf("project-%d", i)), "project")
			if err != nil {
				errs <- err
			}
		}(i)
		go func() {
			defer wait.Done()
			<-start
			_, err := Load()
			if err != nil {
				errs <- err
			}
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	got, err := Load()
	if err != nil || len(got.Projects) != count {
		t.Fatalf("projects=%d err=%v", len(got.Projects), err)
	}
}
