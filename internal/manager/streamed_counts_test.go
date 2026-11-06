package manager_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type changedOutcomeRunner struct{}

func (changedOutcomeRunner) Run(_ context.Context, _ domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	onEvent(runner.Event{Type: runner.EventStdout, Line: "PASSED Alpha"})
	onEvent(runner.Event{Type: runner.EventStdout, Line: "FAILED Alpha"})
	return runner.Result{ExitCode: 1}, errors.New("fixture failed")
}

type recoveredOutcomeRunner struct{}

func (recoveredOutcomeRunner) Run(_ context.Context, _ domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	onEvent(runner.Event{Type: runner.EventStdout, Line: "FAILED Alpha"})
	onEvent(runner.Event{Type: runner.EventStdout, Line: "PASSED Alpha"})
	return runner.Result{}, nil
}

func TestStreamedRecoveryReplacesPriorFailure(t *testing.T) {
	provider := domain.Provider{ID: "fixture", Name: "Fixture", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "streamed-counts", Providers: []domain.Provider{provider}}, recoveredOutcomeRunner{})
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
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 1 || diag.Run.Failed != 0 {
		t.Fatalf("recovered outcome must replace previous failure: %+v, %v", diag.Run, err)
	}
}

func TestStreamedOutcomeChangeReplacesPriorCount(t *testing.T) {
	provider := domain.Provider{ID: "fixture", Name: "Fixture", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "streamed-counts", Providers: []domain.Provider{provider}}, changedOutcomeRunner{})
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
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed || diag.Run.Passed != 0 || diag.Run.Failed != 1 {
		t.Fatalf("updated outcome must replace previous count: %+v, %v", diag.Run, err)
	}
}

func TestStreamedCountsProcessHelper(t *testing.T) {
	if os.Getenv("TESTDESK_STREAM_COUNTS_HELPER") != "1" {
		return
	}
	fmt.Println("PASSED Alpha")
	fmt.Println("PASSED Alpha")
	fmt.Println("PASSED Beta")
}

func TestProviderWideStreamedCountsWithRealProcesses(t *testing.T) {
	command := domain.CommandSpec{
		Exe: os.Args[0], Args: []string{"-test.run=^TestStreamedCountsProcessHelper$"},
		Env: map[string]string{"TESTDESK_STREAM_COUNTS_HELPER": "1"},
	}
	providers := []domain.Provider{
		{ID: "first", Name: "First", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: command}},
		{ID: "second", Name: "Second", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: command}},
	}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "streamed-counts", Providers: providers}, runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.Run(context.Background(), manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 2048)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 4 {
		t.Fatalf("two processes each reported two passes: %+v, %v", diag.Run, err)
	}
}

type repeatedNamesRunner struct{}

func (repeatedNamesRunner) Run(_ context.Context, _ domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	for _, name := range []string{"Alpha", "Alpha", "Beta"} {
		onEvent(runner.Event{Type: runner.EventStdout, Line: "PASSED " + name})
	}
	return runner.Result{}, nil
}

func TestProviderWideStreamedCountsAreScopedToEachItem(t *testing.T) {
	providers := []domain.Provider{
		{ID: "first", Name: "First", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "first"}}},
		{ID: "second", Name: "Second", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "second"}}},
	}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "streamed-counts", Providers: providers}, repeatedNamesRunner{})
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
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 4 {
		t.Fatalf("two providers each reported two passes: %+v, %v", diag.Run, err)
	}
}
