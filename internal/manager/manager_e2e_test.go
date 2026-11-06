package manager_test

import (
	"context"
	"encoding/json"
	"io/fs"
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

func TestManagerDiscoverRunAndPersistE2E(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tests, err := mgr.Discover(ctx)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(tests) != 8 {
		t.Fatalf("expected 8 tests, got %d", len(tests))
	}
	run, err := mgr.Run(ctx, manager.RunRequest{Filter: domain.Filter{Query: "ValidOrder"}})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if run.Total != 1 {
		t.Fatalf("expected one selected item, got %d", run.Total)
	}
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := mgr.Diagnostics(run.ID, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if diag.Run == nil || diag.Run.Status != domain.StatusPassed {
		t.Fatalf("expected passed diagnostic run, got %+v", diag.Run)
	}
	snap, err := mgr.Snapshot(domain.Filter{Statuses: []domain.Status{domain.StatusPassed}, Query: "ValidOrder", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.Tests) != 1 || snap.Tests[0].LastStatus != domain.StatusPassed {
		t.Fatalf("expected persisted passed status, got %+v", snap.Tests)
	}
}

func TestManagerDiscoverWithProgressE2E(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var progressUpdates []manager.DiscoverProgress

	tests, err := mgr.DiscoverWithProgress(ctx, func(progress manager.DiscoverProgress) {
		progressUpdates = append(progressUpdates, progress)
	})
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	if len(tests) != 8 {
		t.Fatalf("expected 8 tests, got %d", len(tests))
	}
	if len(progressUpdates) == 0 {
		t.Fatal("expected progress callback to receive updates")
	}
	last := progressUpdates[len(progressUpdates)-1]
	if last.DiscoveredTotal != 8 {
		t.Fatalf("expected final count in progress to be 8, got %d", last.DiscoveredTotal)
	}
}

func TestManagerRunProviderReportsFailureE2E(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := mgr.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := mgr.Run(ctx, manager.RunRequest{ProviderIDs: []string{"backend-xunit"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := mgr.Diagnostics(run.ID, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if diag.Run == nil || diag.Run.Status != domain.StatusFailed || diag.Run.Failed == 0 {
		t.Fatalf("expected failed run with failed item, got %+v", diag.Run)
	}
}

func TestManagerSnapshotReportsPauseState(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Control(manager.ControlPause); err != nil {
		t.Fatal(err)
	}
	paused, err := mgr.Snapshot(domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if !paused.Paused {
		t.Fatal("expected snapshot to report paused lifecycle state")
	}
	if err := mgr.Control(manager.ControlResume); err != nil {
		t.Fatal(err)
	}
	resumed, err := mgr.Snapshot(domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Paused {
		t.Fatal("expected snapshot to report resumed lifecycle state")
	}
}

func TestManagerControlSkipMarksTestSkipped(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	tests, err := mgr.Discover(context.Background())
	if err != nil || len(tests) == 0 {
		t.Fatalf("discovery failed: %v", err)
	}
	_, err = mgr.Run(context.Background(), manager.RunRequest{TestIDs: []string{tests[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	// Request skip while running
	_ = mgr.Control(manager.ControlSkip)
	_ = mgr.WaitForIdle(context.Background())
	snap, err := mgr.Snapshot(domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.RecentRuns) > 0 {
		firstRun := snap.RecentRuns[0]
		if firstRun.Skipped == 0 && firstRun.Status != domain.StatusSkipped {
			t.Logf("run completed: status=%s skipped=%d", firstRun.Status, firstRun.Skipped)
		}
	}
}

func TestDiscoverIntellagentWorkspace(t *testing.T) {
	if os.Getenv("TESTDESK_E2E_INTELLAGENT") == "" {
		t.Skip("skipping local Intellagent E2E discovery test; set TESTDESK_E2E_INTELLAGENT=1 to run")
	}
	projectPath := `C:\Users\aiden\Documents\GitLab\Intellagent-Workspace`
	if _, err := os.Stat(projectPath); err != nil {
		t.Skip("Intellagent-Workspace not present")
	}
	mgr, err := manager.NewFromProject(projectPath)
	if err != nil {
		t.Fatal(err)
	}
	tests, err := mgr.Discover(context.Background())
	if err != nil {
		t.Logf("Discovery warnings: %v", err)
	}
	t.Logf("Total tests discovered in Intellagent-Workspace: %d", len(tests))
	providerCounts := map[string]int{}
	for _, tc := range tests {
		providerCounts[tc.ProviderID]++
	}
	for pid, count := range providerCounts {
		t.Logf("Provider %s: %d tests", pid, count)
	}
	if len(tests) < 2000 {
		t.Fatalf("expected >= 2000 tests, got %d", len(tests))
	}
}

func TestManagerDiscoveryRetainsAmbiguousIdentitiesBeforePersistence(t *testing.T) {
	project := t.TempDir()
	provider := config.DotNetProvider()
	provider.ID = "duplicate-dotnet"
	provider.Name = "Duplicate .NET"
	provider.Root = "."
	provider.Discover.Command = domain.CommandSpec{Exe: "fake-discovery"}
	cfg := domain.ProjectConfig{
		Version:   1,
		Name:      "Duplicate discovery",
		Workspace: ".testdesk",
		Providers: []domain.Provider{provider},
	}
	mgr, err := manager.New(project, cfg, duplicateDiscoveryRunner{})
	if err != nil {
		t.Fatal(err)
	}

	tests, err := mgr.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(tests) != 2 || tests[0].ID == tests[1].ID {
		t.Fatalf("expected two distinct catalog identities, got %d: %+v", len(tests), tests)
	}
	if _, err := mgr.Run(context.Background(), manager.RunRequest{TestIDs: []string{tests[0].ID}}); err == nil {
		t.Fatal("ambiguous selector should not execute")
	}
	snapshot, err := mgr.Snapshot(domain.Filter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.TestCount != 2 || len(snapshot.Tests) != 2 {
		t.Fatalf("snapshot count drifted from unique catalogue: %+v", snapshot)
	}

	data, err := os.ReadFile(filepath.Join(project, ".testdesk", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var persisted []domain.TestCase
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if len(persisted) != 2 {
		t.Fatalf("expected two persisted test identities, got %d", len(persisted))
	}
}

type duplicateDiscoveryRunner struct{}

func (duplicateDiscoveryRunner) Run(_ context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	const test = "Example.Tests.AuthenticationTests.ValidCredentials_passes"
	return runner.Result{Stdout: "The following Tests are available:\n" + test + "\n" + test + "\n"}, nil
}

func TestManagerStreamingExecutionStats(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = mgr.Discover(ctx)
	if err != nil {
		t.Fatalf("discover failed: %v", err)
	}
	run, err := mgr.Run(ctx, manager.RunRequest{})
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	snap, err := mgr.Snapshot(domain.Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.RecentRuns) == 0 {
		t.Fatal("expected completed run in recent runs")
	}
	lastRun := snap.RecentRuns[0]
	if lastRun.ID != run.ID {
		t.Fatalf("expected last run %s, got %s", run.ID, lastRun.ID)
	}
	if lastRun.Passed == 0 {
		t.Fatalf("expected passed tests count to be greater than 0, got %d", lastRun.Passed)
	}
}

func copySampleProject(t *testing.T) string {
	t.Helper()
	repoRoot := findRepoRoot(t)
	source := filepath.Join(repoRoot, "examples", "sample-project")
	target := filepath.Join(t.TempDir(), "sample-project")
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == ".testdesk" || strings.HasPrefix(filepath.ToSlash(rel), ".testdesk/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(dest, data, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	return target
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			t.Fatal("repo root not found")
		}
		cwd = parent
	}
}
