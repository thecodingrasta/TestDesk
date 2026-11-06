package config

import (
	"reflect"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestLegacyGoProviderMigrationPreservesCustomCommands(t *testing.T) {
	legacy := GoTestProvider()
	legacy.Discover.Parser = "go-list"
	legacy.Discover.Command.Args = []string{"test", "./...", "-list", "."}
	legacy.Run.Test.Args = []string{"test", "./...", "-run", "^{{test.selector}}$", "-json"}
	legacy.Name = "configured"
	legacy.Run.Test.TimeoutSeconds = 73
	cfg := Normalise("fixture", domain.ProjectConfig{Providers: []domain.Provider{legacy}})
	got := cfg.Providers[0]
	if got.Name != "configured" || got.Run.Test.TimeoutSeconds != 73 || got.Discover.Parser != "go-json-list" || got.Run.Test.Args[1] != "{{test.goPackage}}" {
		t.Fatalf("legacy migration: %+v", got)
	}
	legacy.Discover.Command.Args = []string{"custom", "discover"}
	legacy.Run.Test.Args = []string{"custom", "run"}
	cfg = Normalise("fixture", domain.ProjectConfig{Providers: []domain.Provider{legacy}})
	if !reflect.DeepEqual(cfg.Providers[0].Discover.Command.Args, legacy.Discover.Command.Args) || !reflect.DeepEqual(cfg.Providers[0].Run.Test.Args, legacy.Run.Test.Args) || cfg.Providers[0].Discover.Parser != "go-list" {
		t.Fatal("custom provider commands must not be rewritten")
	}
}
