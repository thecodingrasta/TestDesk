package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestDetectDotNetProjectsWithSameName(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"first", "second"} {
		path := filepath.Join(root, "tests", dir, "Shared.Tests.csproj")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("<Project />"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Detect(root)
	if len(cfg.Providers) != 2 {
		t.Fatalf("got %d providers, want 2", len(cfg.Providers))
	}
	if err := Validate(cfg); err != nil {
		t.Fatalf("detected configuration must be valid: %v", err)
	}
	if cfg.Providers[0].Root == cfg.Providers[1].Root {
		t.Fatal("distinct projects must retain distinct roots")
	}
	if !reflect.DeepEqual(cfg.Providers, Detect(root).Providers) {
		t.Fatal("detected provider identities must be stable")
	}

	// A saved legacy provider can refer to either project, including the one
	// that sorts second. Its overrides must survive without hiding the other.
	for _, configured := range cfg.Providers {
		configured.ID = "dotnet-shared-tests"
		configured.Name = "user override"
		configured.Discover.Command.TimeoutSeconds = 42
		saved := cfg
		saved.Providers = []domain.Provider{configured}
		if err := Save(root, saved); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(loaded.Providers) != 2 || loaded.Providers[0].Name != "user override" || loaded.Providers[0].Discover.Command.TimeoutSeconds != 42 {
			t.Fatalf("saved override or second project lost: %+v", loaded.Providers)
		}
		if loaded.Providers[0].Root == loaded.Providers[1].Root {
			t.Fatalf("duplicate project roots after merge: %+v", loaded.Providers)
		}
	}
}

func TestDetectDotNetProjectsInSameDirectoryUseProjectFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"First.Tests.csproj", "Second.Tests.csproj"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("<Project />"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Detect(root)
	if len(cfg.Providers) != 2 {
		t.Fatalf("got %d providers, want 2", len(cfg.Providers))
	}
	for _, provider := range cfg.Providers {
		target := provider.Discover.Command.Args[1]
		if filepath.Ext(target) != ".csproj" || provider.Run.All.Args[1] != target || provider.Run.Test.Args[1] != target {
			t.Fatalf("commands do not target one project: %+v", provider)
		}
	}
}

func TestLoadMigratesGeneratedCommandsWhenProjectSharesDirectory(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "First.Tests.csproj")
	if err := os.WriteFile(first, []byte("<Project />"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Detect(root)
	cfg.Providers[0].Name = "saved name"
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "Second.Tests.csproj"), []byte("<Project />"), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Providers) != 2 || loaded.Providers[0].Name != "saved name" || loaded.Providers[0].Discover.Command.Args[1] != "First.Tests.csproj" {
		t.Fatalf("generated commands not migrated safely: %+v", loaded.Providers)
	}
	cfg.Providers[0].Discover.Command.TimeoutSeconds = 42
	if err := Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Providers[0].Discover.Command.TimeoutSeconds != 42 || loaded.Providers[0].Discover.Command.Args[1] != "." {
		t.Fatal("custom command must not be rewritten")
	}
}
