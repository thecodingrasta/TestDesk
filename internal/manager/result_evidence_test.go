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

type evidenceRunner struct{ report string }

func (r evidenceRunner) Run(_ context.Context, cmd domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	if cmd.Exe == "discover" {
		return runner.Result{Stdout: "Fixture.UnitTest.Passes\n"}, nil
	}
	if onEvent != nil {
		onEvent(runner.Event{Type: runner.EventStdout, Line: "PASSED Fixture.UnitTest.Other"})
	}
	if r.report != "" {
		if err := os.MkdirAll(cmd.Args[0], 0o700); err != nil {
			return runner.Result{}, err
		}
		if err := os.WriteFile(filepath.Join(cmd.Args[0], "result.trx"), []byte(r.report), 0o600); err != nil {
			return runner.Result{}, err
		}
	}
	now := time.Now()
	return runner.Result{Started: now, Finished: now}, nil
}

func TestSelectedReportRequiresMatchingResultEvidence(t *testing.T) {
	const pass = `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="Passed"/></Results></TestRun>`
	const other = `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Other" outcome="Passed"/></Results></TestRun>`
	const mixed = `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="Passed"/><UnitTestResult testName="Fixture.UnitTest.Other" outcome="Failed"/></Results></TestRun>`
	for _, tc := range []struct {
		name, report          string
		runStatus, testStatus domain.Status
	}{
		{"missing", "", domain.StatusFailed, domain.StatusUnknown},
		{"malformed", "<TestRun>", domain.StatusFailed, domain.StatusUnknown},
		{"zero", "<TestRun><Results/></TestRun>", domain.StatusFailed, domain.StatusUnknown},
		{"unmatched", other, domain.StatusFailed, domain.StatusUnknown},
		{"unknown outcome", `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="Unknown"/></Results></TestRun>`, domain.StatusFailed, domain.StatusUnknown},
		{"passing", pass, domain.StatusPassed, domain.StatusPassed},
		{"skipped", `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="NotExecuted"/></Results></TestRun>`, domain.StatusSkipped, domain.StatusSkipped},
		{"failed result", `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="Failed"/></Results></TestRun>`, domain.StatusFailed, domain.StatusFailed},
		{"mixed", mixed, domain.StatusFailed, domain.StatusPassed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			p := config.DotNetProvider()
			p.ID, p.Root = "fixture", "."
			p.Discover = domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "discover"}}
			p.Run.Test = domain.CommandSpec{Exe: "run", Args: []string{"{{artifact.dir}}"}}
			cfg := domain.ProjectConfig{Version: 1, Name: "evidence", Providers: []domain.Provider{p}}
			m, err := manager.New(project, cfg, evidenceRunner{report: tc.report})
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
			if err != nil || diag.Run == nil || diag.Run.Status != tc.runStatus {
				t.Fatalf("run: %+v, %v; want %s", diag.Run, err, tc.runStatus)
			}
			if tc.testStatus == domain.StatusUnknown && diag.Run.Passed != 0 {
				t.Fatalf("unmatched streamed result counted as a pass: %+v", diag.Run)
			}
			reopened, err := manager.New(project, cfg, evidenceRunner{})
			if err != nil {
				t.Fatal(err)
			}
			snap, err := reopened.Snapshot(domain.Filter{Limit: 10})
			if err != nil || len(snap.Tests) != 1 || snap.Tests[0].LastStatus != tc.testStatus {
				t.Fatalf("reopened: %+v, %v; want %s", snap.Tests, err, tc.testStatus)
			}
		})
	}
}

func TestSelectedResultPersistenceFailureFailsRun(t *testing.T) {
	project := t.TempDir()
	p := config.DotNetProvider()
	p.ID, p.Root = "fixture", "."
	p.Discover = domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "discover"}}
	p.Run.Test = domain.CommandSpec{Exe: "run", Args: []string{"{{artifact.dir}}"}}
	cfg := domain.ProjectConfig{Version: 1, Name: "evidence", Providers: []domain.Provider{p}}
	proc := evidenceRunner{report: `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="Passed"/></Results></TestRun>`}
	m, err := manager.New(project, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	tests, err := m.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	catalog := filepath.Join(project, ".testdesk", "catalog.json")
	if err := os.Remove(catalog); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(catalog, 0o700); err != nil {
		t.Fatal(err)
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
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed {
		t.Fatalf("run: %+v, %v", diag.Run, err)
	}
	snapshot, err := m.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 1 || snapshot.Tests[0].LastStatus != domain.StatusUnknown {
		t.Fatalf("unpersisted passing result appeared in catalog: %+v %v", snapshot.Tests, err)
	}
}

func TestProviderRunWithOnlySkippedResultsIsNotPassed(t *testing.T) {
	project := t.TempDir()
	p := config.DotNetProvider()
	p.ID, p.Root = "fixture", "."
	p.Run.All = domain.CommandSpec{Exe: "run", Args: []string{"{{artifact.dir}}"}}
	proc := evidenceRunner{report: `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="NotExecuted"/></Results></TestRun>`}
	m, err := manager.New(project, domain.ProjectConfig{Version: 1, Name: "evidence", Providers: []domain.Provider{p}}, proc)
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
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusSkipped {
		t.Fatalf("run: %+v, %v", diag.Run, err)
	}
}
