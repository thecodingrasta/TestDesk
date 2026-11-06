package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type statusDiscoveryRunner struct{ started chan struct{} }

func (r statusDiscoveryRunner) Run(ctx context.Context, _ domain.CommandSpec, emit func(runner.Event)) (runner.Result, error) {
	emit(runner.Event{Type: runner.EventStderr, Line: "sensitive text that must not be exposed"})
	close(r.started)
	<-ctx.Done()
	return runner.Result{}, ctx.Err()
}
func TestDiscoveryStatusTracksObservedOutputAndCancellation(t *testing.T) {
	fixture := statusDiscoveryRunner{started: make(chan struct{})}
	p := domain.Provider{ID: "slow", Name: "Slow provider", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: []domain.Provider{p}}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if state := m.DiscoveryStatus(); state.Active || state.Outcome != "idle" {
		t.Fatalf("initial status: %+v", state)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := m.Discover(ctx); done <- err }()
	select {
	case <-fixture.started:
	case <-time.After(3 * time.Second):
		t.Fatal("discovery did not start")
	}
	s := New(m)
	reply, err := s.callTool(context.Background(), json.RawMessage(`{"name":"testdesk_discovery_status","arguments":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(reply)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Slow provider") || strings.Contains(string(raw), "sensitive text") {
		t.Fatalf("status payload: %s", raw)
	}
	state := m.DiscoveryStatus()
	if !state.Active || state.Outcome != "running" || state.LastOutputAt == nil || state.QuietMillis == nil || state.ProviderID != "slow" {
		t.Fatalf("running status: %+v", state)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation did not finish")
	}
	state = m.DiscoveryStatus()
	if state.Active || state.Outcome != "cancelled" || state.FinishedAt == nil {
		t.Fatalf("cancelled status: %+v", state)
	}
	other, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "other"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if state := other.DiscoveryStatus(); state.Active || state.Outcome != "idle" {
		t.Fatalf("other project status: %+v", state)
	}
}

type statusResultRunner struct{ fail bool }

func (r statusResultRunner) Run(context.Context, domain.CommandSpec, func(runner.Event)) (runner.Result, error) {
	if r.fail {
		return runner.Result{ExitCode: 1}, nil
	}
	return runner.Result{Stdout: "fixture.case\n"}, nil
}
func TestDiscoveryStatusReportsCompletionAndFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		fail bool
		want string
	}{{"success", false, "completed"}, {"failure", true, "failed"}} {
		t.Run(tc.name, func(t *testing.T) {
			p := domain.Provider{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
			m, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: []domain.Provider{p}}, statusResultRunner{fail: tc.fail})
			if err != nil {
				t.Fatal(err)
			}
			_, err = m.Discover(context.Background())
			if (err != nil) != tc.fail {
				t.Fatalf("discovery err=%v", err)
			}
			status := m.DiscoveryStatus()
			if status.Active || status.Outcome != tc.want || status.FinishedAt == nil {
				t.Fatalf("status=%+v", status)
			}
		})
	}
}
