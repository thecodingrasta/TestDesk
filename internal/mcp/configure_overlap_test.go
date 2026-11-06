package mcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestConfigureDoesNotAppendEquivalentDotNetProvider(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.csproj"), []byte(`<Project><PropertyGroup><IsTestProject>true</IsTestProject></PropertyGroup></Project>`), 0600); err != nil {
		t.Fatal(err)
	}
	p := config.DotNetProvider()
	p.ID = "fixture"
	cfg := config.Normalise(root, domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{p}})
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	m, err := manager.New(root, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	s := New(m)
	result, err := s.configureProject(map[string]any{"project": root})
	if err != nil {
		t.Fatal(err)
	}
	if got := m.Config().Providers; len(got) != 1 || got[0].ID != "fixture" {
		t.Fatalf("duplicate providers: %+v; result=%+v", got, result)
	}
}
