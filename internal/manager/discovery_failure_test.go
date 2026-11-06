package manager_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type discoveryFailureRunner struct {
	result runner.Result
	err    error
}

func (r *discoveryFailureRunner) Run(context.Context, domain.CommandSpec, func(runner.Event)) (runner.Result, error) {
	return r.result, r.err
}

func TestDiscoveryFailureRetainsPreviousProviderCatalog(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		timeout      bool
	}{
		{"exit-empty", "", false}, {"exit-partial", "partial\n", false},
		{"timeout-empty", "", true}, {"timeout-partial", "partial\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			project := t.TempDir()
			provider := config.DotNetProvider()
			provider.ID, provider.Root = "fixture", "."
			provider.Discover.Parser = "lines"
			provider.Discover.Command = domain.CommandSpec{Exe: "fixture"}
			cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Workspace: ".testdesk", Providers: []domain.Provider{provider}}
			proc := &discoveryFailureRunner{result: runner.Result{Stdout: "first\nsecond\n"}}
			mgr, err := manager.New(project, cfg, proc)
			if err != nil {
				t.Fatal(err)
			}
			if tests, err := mgr.Discover(context.Background()); err != nil || len(tests) != 2 {
				t.Fatalf("initial discovery: %d, %v", len(tests), err)
			}
			proc.result = runner.Result{Stdout: tc.output, ExitCode: 1, TimedOut: tc.timeout}
			if tc.output == "" {
				proc.err = errors.New("fixture failure")
			}
			failedProgress := false
			tests, err := mgr.DiscoverWithProgress(context.Background(), func(p manager.DiscoverProgress) {
				if p.Phase == "provider_failed" {
					failedProgress = true
				}
			})
			if err == nil || !strings.Contains(err.Error(), "fixture") || len(tests) != 2 || !failedProgress {
				t.Fatalf("failure must retain catalog and report status: tests=%v err=%v progress=%v", tests, err, failedProgress)
			}
			reopened, err := manager.New(project, cfg, proc)
			if err != nil {
				t.Fatal(err)
			}
			snap, err := reopened.Snapshot(domain.Filter{Limit: 10})
			if err != nil || len(snap.Tests) != 2 {
				t.Fatalf("restart: %d tests, %v", len(snap.Tests), err)
			}
			for _, test := range snap.Tests {
				if test.Name == "partial" {
					t.Fatal("partial output replaced known-good catalog")
				}
			}
		})
	}
}

func TestDiscoveryRetainsCatalogWhenDotNetReportsExceptionWithZeroExit(t *testing.T) {
	project := t.TempDir()
	provider := config.DotNetProvider()
	provider.ID, provider.Root = "fixture", "."
	provider.Discover.Command = domain.CommandSpec{Exe: "fixture"}
	cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{provider}}
	proc := &discoveryFailureRunner{result: runner.Result{Stdout: "The following Tests are available:\n  Example.Tests.Known\n"}}
	mgr, err := manager.New(project, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	if tests, err := mgr.Discover(context.Background()); err != nil || len(tests) != 1 {
		t.Fatalf("initial discovery: %v, %v", tests, err)
	}
	proc.result = runner.Result{Stdout: "The following Tests are available:\n  Example.Tests.Partial\nMicrosoft.VisualStudio.TestPlatform.ObjectModel.TestPlatformException: vstest.console process failed to connect to testhost process\n"}
	tests, err := mgr.Discover(context.Background())
	if err == nil || len(tests) != 1 || tests[0].FullName != "Example.Tests.Known" {
		t.Fatalf("reported failure replaced catalog: %v, %v", tests, err)
	}
	reopened, err := manager.New(project, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 1 || snapshot.Tests[0].FullName != "Example.Tests.Known" {
		t.Fatalf("reopened catalog: %+v, %v", snapshot.Tests, err)
	}
}
