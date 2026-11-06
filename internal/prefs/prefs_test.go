package prefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestUpsertProjectTracksRecentProjects(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	first := filepath.Join(t.TempDir(), "one")
	second := filepath.Join(t.TempDir(), "two")
	if err := os.MkdirAll(first, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(second, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := UpsertProject(first, "One"); err != nil {
		t.Fatal(err)
	}
	if _, err := UpsertProject(second, "Two"); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastProjectRoot != second {
		t.Fatalf("expected second project to be active, got %q", loaded.LastProjectRoot)
	}
	if len(loaded.Projects) != 2 {
		t.Fatalf("expected 2 projects, got %d", len(loaded.Projects))
	}
}

func TestForgetProjectPromotesRemainingProject(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	first := filepath.Join(t.TempDir(), "one")
	second := filepath.Join(t.TempDir(), "two")
	if _, err := UpsertProject(first, "One"); err != nil {
		t.Fatal(err)
	}
	if _, err := UpsertProject(second, "Two"); err != nil {
		t.Fatal(err)
	}
	loaded, err := ForgetProject(second)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastProjectRoot != first {
		t.Fatalf("expected first project to be promoted, got %q", loaded.LastProjectRoot)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0].Name != "One" {
		t.Fatalf("unexpected projects: %+v", loaded.Projects)
	}
}

func TestRenameProjectChangesDisplayNameOnly(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "project")
	if _, err := UpsertProject(root, "Original"); err != nil {
		t.Fatal(err)
	}
	updated, err := RenameProject(root, "Renamed")
	if err != nil {
		t.Fatal(err)
	}
	if len(updated.Projects) != 1 || updated.Projects[0].Name != "Renamed" || updated.Projects[0].Root != root {
		t.Fatalf("rename did not preserve project identity: %+v", updated.Projects)
	}
}

func TestNormaliseSyncsProjectNameFromDiskConfig(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := filepath.Join(t.TempDir(), "MauiX-Conversion")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	// Initial save with old folder name
	if _, err := UpsertProject(root, "MauiX-Conversion"); err != nil {
		t.Fatal(err)
	}

	// Now write testdesk.json with updated name "Intellagent-Workspace"
	configContent := `{"version":1,"name":"Intellagent-Workspace","workspace":".testdesk"}`
	if err := os.WriteFile(filepath.Join(root, "testdesk.json"), []byte(configContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Load should auto-sync name from testdesk.json
	loaded, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Projects) != 1 || loaded.Projects[0].Name != "Intellagent-Workspace" {
		t.Fatalf("expected project name to sync to Intellagent-Workspace, got %+v", loaded.Projects)
	}
}
