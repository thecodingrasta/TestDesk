package config

import (
	"github.com/thecodingrasta/testdesk/internal/domain"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSuppressesEquivalentDetectedDotNetProvider(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "fixture.csproj"), []byte(`<Project><PropertyGroup><IsTestProject>true</IsTestProject></PropertyGroup></Project>`), 0600); err != nil {
		t.Fatal(err)
	}
	p := DotNetProvider()
	p.ID = "fixture"
	if err := Save(root, domain.ProjectConfig{Version: 1, Providers: []domain.Provider{p}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "fixture" {
		t.Fatalf("unexpected providers: %+v", cfg.Providers)
	}
}

func TestLoadPreservesExplicitSolutionAndProjectProviders(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "tests", "Fixture.Tests")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Fixture.Tests.csproj"), []byte(`<Project />`), 0600); err != nil {
		t.Fatal(err)
	}
	solution := DotNetProvider()
	project := dotNetProviderForProject("tests/Fixture.Tests/Fixture.Tests.csproj")
	project.Name = "Explicit project"
	if err := Save(root, domain.ProjectConfig{Version: 1, Providers: []domain.Provider{solution, project}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Providers) != 2 || cfg.Providers[0].ID != "dotnet" || cfg.Providers[1].Name != "Explicit project" {
		t.Fatalf("explicit providers changed: %+v", cfg.Providers)
	}
}

func TestDetectedDotNetScopeRetainsDistinctCommands(t *testing.T) {
	a := DotNetProvider()
	a.ID = "custom"
	a.Run.Test.Args = append(a.Run.Test.Args, "--framework", "net8.0")
	b := dotNetProviderForProject("fixture.csproj")
	if sameDetectedDotNetScope(a, b) {
		t.Fatal("distinct command incorrectly treated as duplicate")
	}
}
