package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestNormaliseEnforcesSafeDesktopSizes(t *testing.T) {
	// Medium (default)
	cfg := Normalise("/tmp/example", domain.ProjectConfig{
		UI: domain.UIConfig{
			WidgetMode:    domain.VisibilityAlways,
			CompactWidth:  10,
			CompactHeight: 10,
			ManagerWidth:  320,
			ManagerHeight: 240,
		},
	})
	if cfg.UI.WidgetSize != "medium" {
		t.Fatalf("expected default medium widget size, got %q", cfg.UI.WidgetSize)
	}
	if cfg.UI.CompactWidth != 480 || cfg.UI.CompactHeight != 106 {
		t.Fatalf("medium compact dimensions invalid: %dx%d", cfg.UI.CompactWidth, cfg.UI.CompactHeight)
	}

	// Small
	cfgSmall := Normalise("/tmp/example", domain.ProjectConfig{
		UI: domain.UIConfig{
			WidgetSize:   "small",
			CompactWidth: 10,
		},
	})
	if cfgSmall.UI.WidgetSize != "small" || cfgSmall.UI.CompactWidth != 380 || cfgSmall.UI.CompactHeight != 96 {
		t.Fatalf("small compact dimensions invalid: %+v", cfgSmall.UI)
	}

	// Large
	cfgLarge := Normalise("/tmp/example", domain.ProjectConfig{
		UI: domain.UIConfig{
			WidgetSize:   "large",
			CompactWidth: 10,
		},
	})
	if cfgLarge.UI.WidgetSize != "large" || cfgLarge.UI.CompactWidth != 580 || cfgLarge.UI.CompactHeight != 118 {
		t.Fatalf("large compact dimensions invalid: %+v", cfgLarge.UI)
	}

	if cfg.UI.ManagerWidth < 1250 || cfg.UI.ManagerHeight < 825 {
		t.Fatalf("manager size was not normalised: %dx%d", cfg.UI.ManagerWidth, cfg.UI.ManagerHeight)
	}
}

func TestNormalisePreservesAlwaysOnTopChoice(t *testing.T) {
	cfg := Normalise("/tmp/example", domain.ProjectConfig{
		UI: domain.UIConfig{WidgetMode: domain.VisibilityAlways, AlwaysOnTop: false},
	})
	if cfg.UI.AlwaysOnTop {
		t.Fatal("always-on-top choice was silently changed")
	}
}

func TestDetectIncludesCypressProvider(t *testing.T) {
	root := t.TempDir()
	packageJSON := []byte(`{"devDependencies":{"cypress":"^15.0.0"}}`)
	if err := os.WriteFile(filepath.Join(root, "package.json"), packageJSON, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Detect(root)
	for _, provider := range cfg.Providers {
		if provider.ID == "cypress" {
			if provider.Kind != domain.ProviderCypress || provider.Run.All.IsZero() || provider.Results.Format != "junit" {
				t.Fatalf("invalid Cypress provider: %+v", provider)
			}
			return
		}
	}
	t.Fatal("Cypress dependency was not detected")
}

func TestDetectIncludesDotNetProviderForSlnxAndNestedProjects(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Intellagent.Native.slnx"), []byte("<Solution />"), 0o600); err != nil {
		t.Fatal(err)
	}
	projectDir := filepath.Join(root, "tests", "Example.Tests")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "Example.Tests.csproj"), []byte("<Project />"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Detect(root)
	if len(cfg.Providers) != 1 || cfg.Providers[0].ID != "dotnet-example-tests" || cfg.Providers[0].Root != "tests/Example.Tests" || strings.Join(cfg.Providers[0].Discover.Command.Args, " ") != "test . --list-tests --nologo" {
		t.Fatalf("expected a project-scoped .NET provider, got %+v", cfg.Providers)
	}
}

func TestLoadPreservesGenericDotNetProviderWhenProjectsAreDetected(t *testing.T) {
	root := t.TempDir()
	for _, relative := range []string{"tests/Unit/Unit.Tests.csproj", "tests/Windows/Windows.Tests.csproj"} {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("<Project />"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	legacy := Normalise(root, domain.ProjectConfig{Version: 1, Providers: []domain.Provider{DotNetProvider()}})
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), data, 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 3 || loaded.Providers[0].ID != "dotnet" || loaded.Providers[1].ID != "dotnet-unit-tests" || loaded.Providers[2].ID != "dotnet-windows-tests" {
		t.Fatalf("expected original and detected providers, got %+v", loaded.Providers)
	}
	for _, provider := range loaded.Providers[1:] {
		if strings.Join(provider.Discover.Command.Args, " ") != "test . --list-tests --nologo" {
			t.Fatalf("expected migrated discovery command to run in the project directory, got %+v", provider.Discover.Command)
		}
	}
}

func TestLoadAddsProviderIntroducedByFrameworkConversion(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Intellagent.Native.slnx"), []byte("<Solution />"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := Normalise(root, domain.ProjectConfig{
		Version:   1,
		Name:      "Converted project",
		Workspace: ".testdesk",
		Providers: []domain.Provider{},
	})
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), data, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 || loaded.Providers[0].ID != "dotnet" {
		t.Fatalf("expected converted project to gain the detected .NET provider, got %+v", loaded.Providers)
	}
}

func TestLoadPreservesConfiguredProviderOverrides(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "Example.slnx"), []byte("<Solution />"), 0o600); err != nil {
		t.Fatal(err)
	}
	configured := DotNetProvider()
	configured.Enabled = false
	configured.Name = "Custom .NET configuration"
	cfg := Normalise(root, domain.ProjectConfig{
		Version:   1,
		Name:      "Configured project",
		Workspace: ".testdesk",
		Providers: []domain.Provider{configured},
	})
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, FileName), data, 0o600); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 1 || loaded.Providers[0].Enabled || loaded.Providers[0].Name != configured.Name {
		t.Fatalf("configured provider was replaced by detection: %+v", loaded.Providers)
	}
}

func TestNormaliseMigratesLegacyAlwaysOnTop(t *testing.T) {
	always := Normalise("/tmp/example", domain.ProjectConfig{UI: domain.UIConfig{AlwaysOnTop: true}})
	if always.UI.TopmostMode != domain.VisibilityAlways || !always.UI.AlwaysOnTop {
		t.Fatalf("legacy true value was not migrated: %+v", always.UI)
	}
	off := Normalise("/tmp/example", domain.ProjectConfig{UI: domain.UIConfig{AlwaysOnTop: false}})
	if off.UI.TopmostMode != domain.VisibilityOff || off.UI.AlwaysOnTop {
		t.Fatalf("legacy false value was not migrated: %+v", off.UI)
	}
	active := Normalise("/tmp/example", domain.ProjectConfig{UI: domain.UIConfig{TopmostMode: domain.VisibilityWhenActive}})
	if active.UI.TopmostMode != domain.VisibilityWhenActive || active.UI.AlwaysOnTop {
		t.Fatalf("active-only value was not preserved: %+v", active.UI)
	}
}

func TestValidateRejectsInvalidWindowModes(t *testing.T) {
	cfg := Normalise("/tmp/example", domain.ProjectConfig{})
	cfg.UI.WidgetMode = domain.VisibilityMode("sometimes")
	if err := Validate(cfg); err == nil {
		t.Fatal("invalid widget mode was accepted")
	}
	cfg = Normalise("/tmp/example", domain.ProjectConfig{})
	cfg.UI.TopmostMode = domain.VisibilityMode("sometimes")
	if err := Validate(cfg); err == nil {
		t.Fatal("invalid topmost mode was accepted")
	}
}

func TestSaveAddsTestDeskWorkspaceToExistingGitIgnore(t *testing.T) {
	root := t.TempDir()
	ignorePath := filepath.Join(root, ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("bin/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, Detect(root)); err != nil {
		t.Fatal(err)
	}
	if err := Save(root, Detect(root)); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(content), ".testdesk/"); count != 1 {
		t.Fatalf("expected one TestDesk workspace ignore entry, got %d: %s", count, content)
	}
	if !strings.Contains(string(content), "TestDesk generated test state") {
		t.Fatalf("missing TestDesk attribution comment: %s", content)
	}
}
