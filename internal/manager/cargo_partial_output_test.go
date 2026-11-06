package manager_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type cargoPartialOutputRunner struct{}

func (cargoPartialOutputRunner) Run(_ context.Context, cmd domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	if cmd.Exe == "discover" {
		return runner.Result{Stdout: "known\n"}, nil
	}
	return runner.Result{Stdout: "test known ... ok\n" + strings.Repeat("x", 4*1024*1024+1) + "\n"}, nil
}

func TestCargoPartialOutputDoesNotPersistPassingEvidence(t *testing.T) {
	for _, targeted := range []bool{false, true} {
		t.Run(map[bool]string{false: "provider", true: "selected"}[targeted], func(t *testing.T) {
			root := t.TempDir()
			provider := domain.Provider{ID: "fixture", Kind: domain.ProviderCustom, Enabled: true,
				Discover: domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "discover"}},
				Run:      domain.RunSpec{All: domain.CommandSpec{Exe: "run"}, Test: domain.CommandSpec{Exe: "run"}},
				Results:  domain.ResultSpec{Format: "cargo-text"}}
			cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{provider}}
			m, err := manager.New(root, cfg, cargoPartialOutputRunner{})
			if err != nil {
				t.Fatal(err)
			}
			cases, err := m.Discover(context.Background())
			if err != nil || len(cases) != 1 {
				t.Fatalf("discovery: %v, %v", cases, err)
			}
			request := manager.RunRequest{}
			if targeted {
				request.TestIDs = []string{cases[0].ID}
			}
			run, err := m.Run(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := m.WaitForIdle(ctx); err != nil {
				t.Fatal(err)
			}
			diag, err := m.Diagnostics(run.ID, 1024)
			if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed || diag.Run.Passed != 0 {
				t.Fatalf("partial run accepted: %+v, %v", diag.Run, err)
			}
			reopened, err := manager.New(root, cfg, cargoPartialOutputRunner{})
			if err != nil {
				t.Fatal(err)
			}
			snapshot, err := reopened.Snapshot(domain.Filter{Limit: 10})
			if err != nil || len(snapshot.Tests) != 1 || snapshot.Tests[0].LastStatus != domain.StatusUnknown {
				t.Fatalf("partial result persisted: %+v, %v", snapshot.Tests, err)
			}
		})
	}
}
