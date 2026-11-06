package desktop

import (
	"context"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type visibilityRunner struct{ started chan struct{} }

func (r visibilityRunner) Run(ctx context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	r.started <- struct{}{}
	<-ctx.Done()
	return runner.Result{}, ctx.Err()
}

func TestRunActiveTracksAllLoadedProjects(t *testing.T) {
	newManager := func() (*manager.Manager, chan struct{}) {
		root := t.TempDir()
		provider := config.DotNetProvider()
		provider.Run.All = domain.CommandSpec{Exe: "fixture"}
		provider.Results = domain.ResultSpec{Format: "none"}
		started := make(chan struct{}, 1)
		mgr, err := manager.New(root, domain.ProjectConfig{Version: 1, Providers: []domain.Provider{provider}}, visibilityRunner{started: started})
		if err != nil {
			t.Fatal(err)
		}
		return mgr, started
	}
	first, firstStarted := newManager()
	second, secondStarted := newManager()
	app := &App{projectRoot: "first", managers: map[string]*manager.Manager{"first": first, "second": second}}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	defer cancelSecond()
	if _, err := first.Run(firstCtx, manager.RunRequest{}); err != nil {
		t.Fatal(err)
	}
	if _, err := second.Run(secondCtx, manager.RunRequest{}); err != nil {
		t.Fatal(err)
	}
	for _, started := range []chan struct{}{firstStarted, secondStarted} {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("project did not start")
		}
	}
	if !app.runActive() {
		t.Fatal("concurrent projects were not considered active")
	}
	cancelFirst()
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelWait()
	if err := first.WaitForIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	if !app.runActive() {
		t.Fatal("the remaining non-selected project must keep the widget active")
	}
	cancelSecond()
	if err := second.WaitForIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	if app.runActive() {
		t.Fatal("all projects are idle")
	}
}

func TestActiveProjectsAndControlsFollowSelectedManager(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	app := &App{managers: make(map[string]*manager.Manager)}
	managers := make([]*manager.Manager, 0, 2)
	for _, name := range []string{"first", "second"} {
		root := t.TempDir()
		provider := config.DotNetProvider()
		provider.Run.All = domain.CommandSpec{Exe: "fixture"}
		provider.Results = domain.ResultSpec{Format: "none"}
		cfg := domain.ProjectConfig{Version: 1, Name: name, Providers: []domain.Provider{provider}}
		if err := config.Save(root, cfg); err != nil {
			t.Fatal(err)
		}
		started := make(chan struct{}, 1)
		mgr, err := manager.New(root, cfg, visibilityRunner{started: started})
		if err != nil {
			t.Fatal(err)
		}
		app.managers[root] = mgr
		managers = append(managers, mgr)
		if _, err := mgr.Run(context.Background(), manager.RunRequest{}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("project did not start")
		}
		t.Cleanup(func() {
			_ = mgr.Control(manager.ControlAction("stop"))
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := mgr.WaitForIdle(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	app.projectRoot = managers[0].ProjectRoot()
	app.managers["duplicate"] = managers[0]
	projects := app.ActiveProjects()
	if len(projects) != 2 {
		t.Fatalf("got %d projects, want two unique managers", len(projects))
	}
	for _, project := range projects {
		if !project.Active || project.Selected != samePath(project.Root, managers[0].ProjectRoot()) {
			t.Fatalf("incorrect activity: %+v", project)
		}
	}
	if _, err := app.SwitchProject(managers[1].ProjectRoot(), false); err != nil {
		t.Fatal(err)
	}
	if err := app.Control("stop"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := managers[1].WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	if !managers[0].IsActive() || !app.runActive() {
		t.Fatal("stopping the selected project stopped or hid the other run")
	}
}
