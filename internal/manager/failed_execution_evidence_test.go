package manager_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type failedEvidenceRunner struct {
	report   string
	timedOut bool
}

func (r failedEvidenceRunner) Run(ctx context.Context, cmd domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	result, err := (evidenceRunner{report: r.report}).Run(ctx, cmd, onEvent)
	if err != nil || cmd.Exe == "discover" {
		return result, err
	}
	result.ExitCode = 1
	result.TimedOut = r.timedOut
	return result, errors.New("fixture process failed")
}

func TestFailedCommandDoesNotInventSelectedTestOutcome(t *testing.T) {
	const prefix = `<TestRun><Results><UnitTestResult testName="Fixture.UnitTest.Passes" outcome="`
	const suffix = `"/></Results></TestRun>`
	for _, tc := range []struct {
		name, report string
		exitOnly     bool
		timedOut     bool
		want         domain.Status
	}{
		{"no report", "", false, false, domain.StatusUnknown},
		{"malformed", "<TestRun>", false, false, domain.StatusUnknown},
		{"unmatched", `<TestRun><Results><UnitTestResult testName="Other" outcome="Failed"/></Results></TestRun>`, false, false, domain.StatusUnknown},
		{"matching failure", prefix + "Failed" + suffix, false, false, domain.StatusFailed},
		{"matching pass", prefix + "Passed" + suffix, false, false, domain.StatusPassed},
		{"exit-only failure", "", true, false, domain.StatusFailed},
		{"timeout with report", prefix + "Passed" + suffix, false, true, domain.StatusTimedOut},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			provider := config.DotNetProvider()
			provider.ID, provider.Root = "fixture", "."
			if tc.exitOnly {
				provider.Results.Format = "none"
			}
			provider.Discover = domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "discover"}}
			provider.Run.Test = domain.CommandSpec{Exe: "run", Args: []string{"{{artifact.dir}}"}}
			cfg := domain.ProjectConfig{Version: 1, Name: "failure-evidence", Providers: []domain.Provider{provider}}
			m, err := manager.New(project, cfg, failedEvidenceRunner{report: tc.report, timedOut: tc.timedOut})
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
			if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed {
				t.Fatalf("run: %+v, %v", diag.Run, err)
			}
			reopened, err := manager.New(project, cfg, failedEvidenceRunner{})
			if err != nil {
				t.Fatal(err)
			}
			snap, err := reopened.Snapshot(domain.Filter{Limit: 10})
			if err != nil || len(snap.Tests) != 1 || snap.Tests[0].LastStatus != tc.want {
				t.Fatalf("test status: %+v, %v; want %s", snap.Tests, err, tc.want)
			}
			if tc.want == domain.StatusPassed && snap.Tests[0].LastFailure != "" {
				t.Fatalf("passing test received process failure: %+v", snap.Tests[0])
			}
		})
	}
}
