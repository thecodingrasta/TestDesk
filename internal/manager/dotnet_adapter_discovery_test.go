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

type adapterDiscoveryRunner struct {
	calls         int
	failAlternate bool
}

func (r *adapterDiscoveryRunner) Run(_ context.Context, cmd domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	r.calls++
	alternate := strings.Contains(strings.Join(cmd.Args, " "), "NUnit.DisplayName=FullNameSep")
	if alternate && r.failAlternate {
		return runner.Result{}, errors.New("identity scan failed")
	}
	name := "Fixture.Cases.Known"
	if alternate {
		name = "Fixture:Cases:Known"
	}
	return runner.Result{Stdout: "The following Tests are available:\n  " + name + "\n"}, nil
}

func TestDotNetAdapterFailurePreservesPreviousCatalog(t *testing.T) {
	p := config.DotNetProvider()
	r := &adapterDiscoveryRunner{}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{p}}, r)
	if err != nil {
		t.Fatal(err)
	}
	first, err := m.Discover(context.Background())
	if err != nil || len(first) != 1 {
		t.Fatalf("initial discovery: %+v %v", first, err)
	}
	r.failAlternate = true
	second, err := m.Discover(context.Background())
	if err == nil || len(second) != 1 || second[0].ID != first[0].ID || r.calls != 4 {
		t.Fatalf("failed identity scan replaced catalog: %+v %v calls=%d", second, err, r.calls)
	}
}
