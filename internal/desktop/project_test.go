package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/prefs"
)

func TestAppRenameProjectPersistsDiskConfig(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	cfg := domain.ProjectConfig{
		Version:   1,
		Name:      "Initial-Name",
		Workspace: ".testdesk",
	}
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := prefs.UpsertProject(root, "Initial-Name"); err != nil {
		t.Fatal(err)
	}
	mgr, err := manager.NewFromProject(root)
	if err != nil {
		t.Fatal(err)
	}

	app := &App{
		projectRoot: root,
		managers:    map[string]*manager.Manager{root: mgr},
	}

	state, err := app.RenameProject(root, "Renamed-Workspace")
	if err != nil {
		t.Fatalf("RenameProject failed: %v", err)
	}

	// 1. Verify returned state
	if state.ProjectName != "Renamed-Workspace" {
		t.Fatalf("expected StartupState.ProjectName to be Renamed-Workspace, got %q", state.ProjectName)
	}

	// 2. Verify disk config
	diskCfg, err := config.Load(root)
	if err != nil {
		t.Fatalf("failed to load disk config: %v", err)
	}
	if diskCfg.Name != "Renamed-Workspace" {
		t.Fatalf("expected disk config name to be Renamed-Workspace, got %q", diskCfg.Name)
	}

	// 3. Verify preferences
	p, err := prefs.Load()
	if err != nil {
		t.Fatalf("failed to load prefs: %v", err)
	}
	if len(p.Projects) != 1 || p.Projects[0].Name != "Renamed-Workspace" {
		t.Fatalf("expected prefs project name to be Renamed-Workspace: %+v", p.Projects)
	}

	// 4. Verify in-memory manager
	if mgr.Config().Name != "Renamed-Workspace" {
		t.Fatalf("expected manager config name to be Renamed-Workspace, got %q", mgr.Config().Name)
	}

	// Clean up
	_ = os.RemoveAll(filepath.Join(root, ".testdesk"))
}
