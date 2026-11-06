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

type queueRunner struct {
	started chan int
	calls   int
}

func (r *queueRunner) Run(ctx context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	r.calls++
	r.started <- r.calls
	if r.calls == 1 {
		<-ctx.Done()
		return runner.Result{ExitCode: 1}, ctx.Err()
	}
	return runner.Result{}, nil
}

func queueManager(t *testing.T, proc runner.Runner) *manager.Manager {
	t.Helper()
	p := config.DotNetProvider()
	p.ID, p.Root = "queue", "."
	p.Run.All = domain.CommandSpec{Exe: "fixture"}
	p.Results = domain.ResultSpec{Format: "none"}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "queue", Providers: []domain.Provider{p}}, proc)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestQueuedRunsRetainTheirOwnCallerContexts(t *testing.T) {
	proc := &queueRunner{started: make(chan int, 2)}
	m := queueManager(t, proc)
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	first, err := m.Run(firstCtx, manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.started:
	case <-time.After(3 * time.Second):
		t.Fatal("first run did not start")
	}
	second, err := m.Run(context.Background(), manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	cancelFirst()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id   string
		want domain.Status
	}{{first.ID, domain.StatusCancelled}, {second.ID, domain.StatusPassed}} {
		diag, err := m.Diagnostics(tc.id, 1024)
		if err != nil || diag.Run == nil || diag.Run.Status != tc.want {
			t.Fatalf("run %s: %+v, %v; want %s", tc.id, diag.Run, err, tc.want)
		}
	}
	select {
	case <-proc.started:
	default:
		t.Fatal("second run never started")
	}
}

func TestSecondManagerDoesNotReconcileLiveRun(t *testing.T) {
	root := t.TempDir()
	p := config.DotNetProvider()
	p.ID, p.Root = "queue", "."
	p.Run.All = domain.CommandSpec{Exe: "fixture"}
	p.Results = domain.ResultSpec{Format: "none"}
	cfg := domain.ProjectConfig{Version: 1, Name: "queue", Providers: []domain.Provider{p}}
	proc := &queueRunner{started: make(chan int, 1)}
	first, err := manager.New(root, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	run, err := first.Run(runCtx, manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.started:
	case <-time.After(3 * time.Second):
		t.Fatal("run did not start")
	}
	second, err := manager.New(root, cfg, failIfRun{})
	if err != nil {
		t.Fatal(err)
	}
	diag, err := second.Diagnostics(run.ID, 1024)
	if err != nil || diag.Run == nil || (diag.Run.Status != domain.StatusQueued && diag.Run.Status != domain.StatusRunning) {
		t.Fatalf("live run was changed by second manager: %+v, %v", diag.Run, err)
	}
	cancelRun()
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelWait()
	if err := first.WaitForIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	diag, err = second.Diagnostics(run.ID, 1024)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusCancelled {
		t.Fatalf("completed run: %+v, %v", diag.Run, err)
	}
}

type failIfRun struct{}

func (failIfRun) Run(context.Context, domain.CommandSpec, func(runner.Event)) (runner.Result, error) {
	return runner.Result{}, errors.New("paused work unexpectedly executed")
}

func TestStopAndKillSettlePausedRun(t *testing.T) {
	for _, action := range []manager.ControlAction{manager.ControlStop, manager.ControlKill} {
		t.Run(string(action), func(t *testing.T) {
			m := queueManager(t, failIfRun{})
			if err := m.Control(manager.ControlPause); err != nil {
				t.Fatal(err)
			}
			run, err := m.Run(context.Background(), manager.RunRequest{})
			if err != nil {
				t.Fatal(err)
			}
			second, err := m.Run(context.Background(), manager.RunRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Control(action); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := m.WaitForIdle(ctx); err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{run.ID, second.ID} {
				diag, err := m.Diagnostics(id, 1024)
				if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusCancelled {
					t.Fatalf("run: %+v, %v", diag.Run, err)
				}
			}
		})
	}
}
