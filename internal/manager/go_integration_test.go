package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

func TestRealGoSameNameSelectionIsPackageScoped(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/fixture\n\ngo 1.22\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "second"} {
		dir := filepath.Join(root, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		failure := ""
		if name == "second" {
			failure = `; t.Error("intentional failure")`
		}
		source := "package " + name + "\nimport (\"os\"; \"testing\")\nfunc TestShared(t *testing.T) { if err := os.WriteFile(\"ran\", []byte(\"yes\"), 0600); err != nil { t.Fatal(err) }" + failure + " }\n"
		if err := os.WriteFile(filepath.Join(dir, "fixture_test.go"), []byte(source), 0600); err != nil {
			t.Fatal(err)
		}
	}
	m, err := manager.New(root, config.Detect(root), runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 || cases[0].ID == cases[1].ID {
		t.Fatalf("discovery must retain both package identities: %+v, %v", cases, err)
	}
	first, second := cases[0], cases[1]
	if strings.HasSuffix(first.GoPackage, "/second") {
		first, second = second, first
	}
	run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{first.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 4096)
	if err != nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 1 {
		t.Fatalf("selected result: %+v, %v", diag.Run, err)
	}
	markers := 0
	for _, name := range []string{"first", "second"} {
		if _, err := os.Stat(filepath.Join(root, name, "ran")); err == nil {
			markers++
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
	if markers != 1 {
		t.Fatalf("selected one test but %d package tests executed", markers)
	}
	run, err = m.Run(ctx, manager.RunRequest{TestIDs: []string{second.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err = m.Diagnostics(run.ID, 4096)
	if err != nil || diag.Run.Status != domain.StatusFailed || diag.Run.Failed != 1 {
		t.Fatalf("failing package result: %+v, %v", diag.Run, err)
	}
	reopened, err := manager.New(root, config.Detect(root), runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	statuses := map[string]domain.Status{}
	for _, tc := range snapshot.Tests {
		statuses[tc.ID] = tc.LastStatus
	}
	if statuses[first.ID] != domain.StatusPassed || statuses[second.ID] != domain.StatusFailed {
		t.Fatalf("package statuses after reopen: %+v", statuses)
	}
}
