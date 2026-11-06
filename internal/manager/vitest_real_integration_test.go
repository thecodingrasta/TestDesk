package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

// Set TESTDESK_VITEST_FIXTURE to an isolated installed Vitest fixture whose
// two named projects expose the same test file and case.
func TestRealVitestNamedProjectsDiscoverAndRemainDistinct(t *testing.T) {
	fixture := os.Getenv("TESTDESK_VITEST_FIXTURE")
	if fixture == "" {
		t.Skip("requires a disposable Vitest fixture")
	}
	marker := filepath.Join(t.TempDir(), "marker.txt")
	t.Setenv("MARKER", marker)
	provider := config.VitestProvider()
	provider.Discover.Command = domain.CommandSpec{Exe: "node", Args: []string{filepath.Join(fixture, "node_modules", "vitest", "vitest.mjs"), "list", "--json"}, WorkingDir: fixture, TimeoutSeconds: 60}
	provider.Run.All = domain.CommandSpec{Exe: "node", Args: []string{filepath.Join(fixture, "node_modules", "vitest", "vitest.mjs"), "run", "--reporter=junit", "--outputFile={{artifact.path}}"}, WorkingDir: fixture, TimeoutSeconds: 60}
	provider.Run.Test.WorkingDir = fixture
	project := t.TempDir()
	cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{provider}}
	mgr, err := manager.New(project, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	tests, err := mgr.Discover(context.Background())
	if err != nil || len(tests) != 6 {
		t.Fatalf("discovery: %+v, %v", tests, err)
	}
	if _, err := mgr.Plan(manager.RunRequest{TestIDs: []string{tests[0].ID}}); err != nil {
		t.Fatalf("exact plan: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	var selected domain.TestCase
	for _, test := range tests {
		if test.VitestProject == "alpha" && filepath.Base(test.File) == "case.test.js" && test.Name == "same [a]+" {
			selected = test
		}
	}
	if selected.ID == "" {
		t.Fatal("alpha test missing")
	}
	selectedRun, err := mgr.Run(ctx, manager.RunRequest{TestIDs: []string{selected.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	selectedDiagnostics, err := mgr.Diagnostics(selectedRun.ID, 4096)
	if err != nil || selectedDiagnostics.Run.Status != domain.StatusPassed || selectedDiagnostics.Run.Passed != 1 {
		t.Fatalf("exact run: %+v, %v", selectedDiagnostics.Run, err)
	}
	contents, err := os.ReadFile(marker)
	if err != nil || string(contents) != "alpha:target\n" {
		t.Fatalf("unexpected test bodies executed: %q, %v", contents, err)
	}
	run, err := mgr.Run(ctx, manager.RunRequest{ProviderIDs: []string{provider.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diagnostics, err := mgr.Diagnostics(run.ID, 4096)
	if err != nil || diagnostics.Run.Status != domain.StatusPassed || diagnostics.Run.Passed != 6 {
		t.Fatalf("provider run: %+v, %v", diagnostics.Run, err)
	}
	reopened, err := manager.New(project, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 6 {
		t.Fatalf("reopened catalog: %+v, %v", snapshot.Tests, err)
	}
	for _, test := range snapshot.Tests {
		if test.LastStatus != domain.StatusUnknown {
			t.Fatalf("ambiguous project result attributed to one catalog case: %+v", snapshot.Tests)
		}
	}
}
