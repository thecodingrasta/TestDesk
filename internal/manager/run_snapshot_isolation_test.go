package manager_test

import (
	"context"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type blockedSnapshotRunner struct {
	started chan struct{}
	release chan struct{}
}

func (r blockedSnapshotRunner) Run(ctx context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	close(r.started)
	select {
	case <-r.release:
		return runner.Result{}, nil
	case <-ctx.Done():
		return runner.Result{}, ctx.Err()
	}
}

func TestRunResponseDoesNotShareMutableQueueItems(t *testing.T) {
	proc := blockedSnapshotRunner{started: make(chan struct{}), release: make(chan struct{})}
	provider := domain.Provider{ID: "fixture", Name: "Fixture", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "snapshot-isolation", Providers: []domain.Provider{provider}}, proc)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	run, err := m.Run(ctx, manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		close(proc.release)
		if err := m.WaitForIdle(ctx); err != nil {
			t.Errorf("run did not finish: %v", err)
		}
	}()
	select {
	case <-proc.started:
	case <-ctx.Done():
		t.Fatal("run did not start")
	}
	if len(run.Items) != 1 || run.Items[0].Status != domain.StatusQueued {
		t.Fatalf("run response changed after dispatch: %+v", run.Items)
	}
	run.Items[0].TestName = "caller-modified"
	snap, err := m.Snapshot(domain.Filter{Limit: 1})
	if err != nil || snap.Running == nil || snap.Running.Items[0].TestName == "caller-modified" {
		t.Fatalf("caller changed manager-owned state: %+v, %v", snap.Running, err)
	}
}
