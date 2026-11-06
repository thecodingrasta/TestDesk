package desktop

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestLoadingStateLifecycle(t *testing.T) {
	app := &App{}

	app.beginLoading("switch-project", "Switching project", "Large workspace")
	started := app.LoadingState()
	if !started.Active || started.Operation != "switch-project" || started.Title != "Switching project" {
		t.Fatalf("unexpected started loading state: %+v", started)
	}
	if started.StartedAt == "" {
		t.Fatal("active loading state must include a start timestamp")
	}

	app.updateLoadingWithProgress("switch-project", "Loading test catalogue", "Scanning provider 1/2", 50)
	updated := app.LoadingState()
	if !updated.Active || updated.Title != "Loading test catalogue" || updated.Percent != 50 {
		t.Fatalf("unexpected updated loading state: %+v", updated)
	}
	if updated.StartedAt != started.StartedAt {
		t.Fatalf("progress update changed operation start time: started=%q updated=%q", started.StartedAt, updated.StartedAt)
	}

	app.finishLoading("switch-project", "Project ready", "Large workspace")
	finished := app.LoadingState()
	if finished.Active || finished.Title != "Project ready" || finished.StartedAt != "" {
		t.Fatalf("unexpected finished loading state: %+v", finished)
	}
}

func TestDiscoveryProgressDoesNotInventCompletionPercentage(t *testing.T) {
	app := &App{}
	app.beginLoading("discover", "Discovering", "")
	app.reportDiscoverProgress("discover", manager.DiscoverProgress{Current: 1, Total: 2, ProviderTests: 75, DiscoveredTotal: 75, Phase: "scanning", Detail: "Scanning provider"})
	state := app.LoadingState()
	if state.Percent != 0 || state.ProviderTests != 75 || state.TotalTests != 75 || state.Detail != "Scanning provider" {
		t.Fatalf("discovery must report evidence without a guessed percentage: %+v", state)
	}
}

func TestInitialiseCompletesFirstLaunchLoading(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	app := &App{
		initialRoot:          root,
		initialConfigured:    false,
		suggestedProjectRoot: root,
		managers:             map[string]*manager.Manager{},
		loading:              LoadingState{Active: true, Operation: "startup", Title: "Opening TestDesk"},
	}

	response, err := app.Initialise(true)
	if err != nil {
		t.Fatalf("Initialise returned an error: %v", err)
	}
	if response.Startup.Configured {
		t.Fatal("first launch without a config must remain unconfigured")
	}
	if state := app.LoadingState(); state.Active || state.Operation != "startup" {
		t.Fatalf("startup loading did not complete: %+v", state)
	}
}
