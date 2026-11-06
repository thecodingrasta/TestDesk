package config

import (
	"github.com/thecodingrasta/testdesk/internal/domain"
	"reflect"
	"testing"
)

func TestCargoLegacyMigrationPreservesCustomCommands(t *testing.T) {
	legacy := CargoProvider()
	legacy.Discover.Parser = "cargo-list"
	legacy.Discover.Command.Args = []string{"test", "--", "--list"}
	legacy.Run.Test.Args = []string{"test", "{{test.selector}}"}
	legacy.Results.Format = "none"
	migrated := Normalise("fixture", domain.ProjectConfig{Providers: []domain.Provider{legacy}}).Providers[0]
	if migrated.Discover.Parser != "cargo-artifacts" || migrated.Results.Format != "cargo-text" {
		t.Fatalf("migration: %+v", migrated)
	}
	legacy.Run.All.Args = []string{"custom"}
	custom := Normalise("fixture", domain.ProjectConfig{Providers: []domain.Provider{legacy}}).Providers[0]
	if !reflect.DeepEqual(custom.Discover, legacy.Discover) || !reflect.DeepEqual(custom.Run, legacy.Run) || !reflect.DeepEqual(custom.Results, legacy.Results) {
		t.Fatal("custom Cargo provider was modified")
	}
}
