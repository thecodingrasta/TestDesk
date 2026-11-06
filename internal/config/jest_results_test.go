package config_test

import (
	"reflect"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestJestResultMigrationPreservesCustomCommands(t *testing.T) {
	generated := config.JestProvider()
	legacy := generated
	legacy.Run.All.Env = nil
	legacy.Run.Test.Env = nil
	legacy.Run.Test.Args = []string{"jest", "{{test.selector}}", "--ci", "--reporters=default", "--reporters=jest-junit"}
	legacy.Results.Path = "junit.xml"
	for _, custom := range []bool{false, true} {
		provider := legacy
		if custom {
			provider.Run.All.TimeoutSeconds++
		}
		got := config.Normalise(t.TempDir(), domain.ProjectConfig{Providers: []domain.Provider{provider}}).Providers[0]
		if custom {
			if !reflect.DeepEqual(got.Run, provider.Run) || got.Results != provider.Results {
				t.Fatalf("custom provider changed: %+v", got)
			}
		} else if !reflect.DeepEqual(got.Run, generated.Run) || got.Results != generated.Results {
			t.Fatalf("generated provider not migrated: %+v", got)
		}
	}
}

func TestJestScopedReportLegacyCommandMigrates(t *testing.T) {
	generated := config.JestProvider()
	legacy := generated
	legacy.Run.All.Env = map[string]string{"JEST_JUNIT_OUTPUT_FILE": "{{artifact.path}}"}
	legacy.Run.Test.Env = map[string]string{"JEST_JUNIT_OUTPUT_FILE": "{{artifact.path}}"}
	legacy.Run.Test.Args = []string{"jest", "{{test.selector}}", "--ci", "--reporters=default", "--reporters=jest-junit"}
	got := config.Normalise(t.TempDir(), domain.ProjectConfig{Providers: []domain.Provider{legacy}}).Providers[0]
	if !reflect.DeepEqual(got.Run, generated.Run) || got.Results != generated.Results {
		t.Fatalf("scoped-report legacy provider not migrated: %+v", got)
	}
}
