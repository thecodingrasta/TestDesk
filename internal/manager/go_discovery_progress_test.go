package manager_test

import (
	"context"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type fragmentedGoDiscoveryRunner struct{}

func (fragmentedGoDiscoveryRunner) Run(_ context.Context, _ domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	lines := []string{
		`{"Action":"start","Package":"example/a"}`,
		`{"Action":"output","Package":"example/a","Output":"TestLong"}`,
		`{"Action":"output","Package":"example/a","Output":"Name\n"}`,
		`{"Action":"pass","Package":"example/a"}`,
	}
	for _, line := range lines {
		onEvent(runner.Event{Type: runner.EventStdout, Line: line})
	}
	return runner.Result{Stdout: lines[0] + "\n" + lines[1] + "\n" + lines[2] + "\n" + lines[3] + "\n"}, nil
}

func TestGoDiscoveryProgressWaitsForCompleteNames(t *testing.T) {
	provider := config.GoTestProvider()
	cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{provider}}
	m, err := manager.New(t.TempDir(), cfg, fragmentedGoDiscoveryRunner{})
	if err != nil {
		t.Fatal(err)
	}
	var observed []string
	_, err = m.DiscoverWithProgress(context.Background(), func(progress manager.DiscoverProgress) {
		if progress.LatestTest != "" {
			observed = append(observed, progress.LatestTest)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range observed {
		if name != "TestLongName" {
			t.Fatalf("reported partial name %q", name)
		}
	}
	if len(observed) == 0 {
		t.Fatal("complete test name was not reported")
	}
}
