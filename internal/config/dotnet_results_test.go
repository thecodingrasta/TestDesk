package config

import (
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestLegacyDotNetResultMigrationPreservesCustomContracts(t *testing.T) {
	p := DotNetProvider()
	p.Results = domain.ResultSpec{Format: "trx", Path: "{{artifact.path}}"}
	p.Run.All.Args = []string{"test", ".", "--logger", "trx;LogFileName={{artifact.name}}"}
	p.Run.Test.Args = []string{"test", ".", "--filter", "FullyQualifiedName=Custom", "--logger", "trx;LogFileName={{artifact.name}}"}
	migrated := migrateLegacyDotNetResults(p)
	if migrated.Results.Format != "trx-dir" || migrated.Results.Path != "{{artifact.dir}}" || !strings.Contains(strings.Join(migrated.Run.Test.Args, " "), "FullyQualifiedName=Custom") {
		t.Fatalf("migration lost results contract or filter: %+v", migrated)
	}
	if strings.Contains(strings.Join(p.Run.All.Args, " "), "--results-directory") {
		t.Fatal("migration mutated input command")
	}
	p.Results.Path = "custom.trx"
	custom := migrateLegacyDotNetResults(p)
	if custom.Results.Path != "custom.trx" || custom.Results.Format != "trx" {
		t.Fatalf("custom report path changed: %+v", custom.Results)
	}
}
