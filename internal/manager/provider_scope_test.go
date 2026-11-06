package manager

import (
	"context"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/runner"
	"sync"
	"testing"
	"time"
)

func TestProviderSelectionIncludesUndiscoveredProviders(t *testing.T) {
	providers := []domain.Provider{
		{ID: "discovered", Name: "discovered", Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-one"}, Test: domain.CommandSpec{Exe: "single"}}},
		{ID: "undiscovered", Name: "undiscovered", Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-two"}}},
		{ID: "disabled", Name: "disabled", Enabled: false, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-three"}}},
	}
	m := &Manager{cfg: domain.ProjectConfig{Providers: providers}, tests: map[string]domain.TestCase{"known": {ID: "known", ProviderID: "discovered", Name: "known"}}}
	items, err := m.Plan(RunRequest{ProviderIDs: []string{"discovered", "undiscovered"}})
	if err != nil || len(items) != 2 {
		t.Fatalf("mixed provider plan: %+v %v", items, err)
	}
	found := map[string]domain.RunItem{}
	for _, item := range items {
		found[item.ProviderID] = item
	}
	if found["discovered"].TestID != "known" || found["undiscovered"].CommandPreview != "all-two" {
		t.Fatalf("incorrect provider scope: %+v", items)
	}
	for _, request := range []RunRequest{{ProviderIDs: []string{"discovered", "missing"}}, {ProviderIDs: []string{"missing"}}, {ProviderIDs: []string{"discovered", "disabled"}}} {
		if items, err := m.Plan(request); err == nil || len(items) != 0 {
			t.Fatalf("unknown provider accepted: %+v %v", items, err)
		}
		if run, err := m.Run(context.Background(), request); err == nil || run.ID != "" {
			t.Fatalf("unknown provider queued: %+v %v", run, err)
		}
	}
}

type providerScopeRunner struct {
	mu    sync.Mutex
	calls []string
}

func (r *providerScopeRunner) Run(_ context.Context, command domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	if command.Exe == "discover" {
		return runner.Result{Stdout: "known\n"}, nil
	}
	r.mu.Lock()
	r.calls = append(r.calls, command.Exe)
	r.mu.Unlock()
	return runner.Result{}, nil
}

func TestProviderSelectionExecutesEachRequestedScope(t *testing.T) {
	proc := &providerScopeRunner{}
	cfg := domain.ProjectConfig{Name: "fixture", Providers: []domain.Provider{
		{ID: "discovered", Name: "discovered", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "discover"}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-one"}, Test: domain.CommandSpec{Exe: "single"}}},
		{ID: "undiscovered", Name: "undiscovered", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-two"}}},
	}}
	m, err := New(t.TempDir(), cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := m.Run(ctx, RunRequest{ProviderIDs: []string{"discovered", "undiscovered", "discovered"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 1024)
	if err != nil || diag.Run.Status != domain.StatusPassed || len(diag.Run.Items) != 2 {
		t.Fatalf("mixed run: %+v %v", diag.Run, err)
	}
	proc.mu.Lock()
	defer proc.mu.Unlock()
	if len(proc.calls) != 2 || proc.calls[0] != "single" || proc.calls[1] != "all-two" {
		t.Fatalf("executed scopes: %v", proc.calls)
	}
}
