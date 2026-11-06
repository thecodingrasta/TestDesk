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
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type partialReportRunner struct{}

func (partialReportRunner) Run(ctx context.Context, cmd domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	const passing = `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="Passed"/></Results></TestRun>`
	result, err := (evidenceRunner{report: passing}).Run(ctx, cmd, onEvent)
	if err != nil || cmd.Exe == "discover" {
		return result, err
	}
	err = os.WriteFile(filepath.Join(cmd.Args[0], "zz-incomplete.trx"), nil, 0o600)
	return result, err
}

func TestSelectedRunRejectsPartialReportDirectory(t *testing.T) {
	project := t.TempDir()
	provider := config.DotNetProvider()
	provider.ID, provider.Root = "fixture", "."
	provider.Discover = domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "discover"}}
	provider.Run.Test = domain.CommandSpec{Exe: "run", Args: []string{"{{artifact.dir}}"}}
	cfg := domain.ProjectConfig{Version: 1, Name: "partial-report", Providers: []domain.Provider{provider}}
	m, err := manager.New(project, cfg, partialReportRunner{})
	if err != nil {
		t.Fatal(err)
	}
	tests, err := m.Discover(context.Background())
	if err != nil || len(tests) != 1 {
		t.Fatalf("discovery: %v %v", tests, err)
	}
	run, err := m.Run(context.Background(), manager.RunRequest{TestIDs: []string{tests[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 2048)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed || diag.Run.Passed != 0 {
		t.Fatalf("partial reports should not yield a pass: %+v, %v", diag.Run, err)
	}
	reopened, err := manager.New(project, cfg, partialReportRunner{})
	if err != nil {
		t.Fatal(err)
	}
	snap, err := reopened.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snap.Tests) != 1 || snap.Tests[0].LastStatus != domain.StatusUnknown {
		t.Fatalf("partial report result after reopening: %+v, %v", snap.Tests, err)
	}
}

func TestProviderRunRejectsPartialReportDirectory(t *testing.T) {
	project := t.TempDir()
	provider := config.DotNetProvider()
	provider.ID, provider.Root = "fixture", "."
	provider.Run.All = domain.CommandSpec{Exe: "run", Args: []string{"{{artifact.dir}}"}}
	m, err := manager.New(project, domain.ProjectConfig{Version: 1, Name: "partial-report", Providers: []domain.Provider{provider}}, partialReportRunner{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.Run(context.Background(), manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 2048)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed || diag.Run.Passed != 0 {
		t.Fatalf("partial reports should not yield a pass: %+v, %v", diag.Run, err)
	}
}
