package manager

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type gatedDiscoveryRunner struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (r *gatedDiscoveryRunner) Run(ctx context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	r.mu.Lock()
	r.calls++
	call := r.calls
	r.mu.Unlock()
	if call > 1 {
		close(r.started)
		select {
		case <-r.release:
		case <-ctx.Done():
			return runner.Result{}, ctx.Err()
		}
	}
	return runner.Result{Stdout: "Fixture.Test\n"}, nil
}

func TestDiscoverySerializesAndPreservesConcurrentRunStatus(t *testing.T) {
	r := &gatedDiscoveryRunner{started: make(chan struct{}), release: make(chan struct{})}
	provider := domain.Provider{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	mgr, err := New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{provider}}, r)
	if err != nil {
		t.Fatal(err)
	}
	tests, err := mgr.Discover(context.Background())
	if err != nil || len(tests) != 1 {
		t.Fatalf("initial discovery: %v, %v", tests, err)
	}
	done := make(chan error, 1)
	go func() { _, err := mgr.Discover(context.Background()); done <- err }()
	select {
	case <-r.started:
	case <-time.After(3 * time.Second):
		t.Fatal("discovery did not start")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := mgr.Discover(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting discovery: %v", err)
	}
	mgr.mu.Lock()
	updated := mgr.tests[tests[0].ID]
	updated.LastStatus = domain.StatusPassed
	updated.LastRunID = "recent-run"
	mgr.tests[updated.ID] = updated
	if err := mgr.store.SaveCatalog(mgr.testsSliceLocked()); err != nil {
		mgr.mu.Unlock()
		t.Fatal(err)
	}
	mgr.mu.Unlock()
	close(r.release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("discovery did not finish")
	}
	stored, err := mgr.store.LoadCatalog()
	if err != nil || len(stored) != 1 || stored[0].LastStatus != domain.StatusPassed || stored[0].LastRunID != "recent-run" {
		t.Fatalf("latest run status was lost: %v, %v", stored, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls != 2 {
		t.Fatalf("cancelled discovery started a provider: %d calls", r.calls)
	}
}
