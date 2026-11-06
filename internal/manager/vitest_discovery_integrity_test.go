package manager_test

import (
	"context"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

func TestVitestMalformedDiscoveryRetainsCatalogAfterRestart(t *testing.T) {
	project := t.TempDir()
	provider := config.VitestProvider()
	provider.Discover.Command = domain.CommandSpec{Exe: "fixture"}
	cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{provider}}
	proc := &discoveryFailureRunner{result: runner.Result{Stdout: `[{"name":"known","type":"test"}]`}}
	mgr, err := manager.New(project, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	if tests, err := mgr.Discover(context.Background()); err != nil || len(tests) != 1 {
		t.Fatalf("initial discovery: %v, %v", tests, err)
	}
	for _, output := range []string{`[{"name":"partial"`, `[{"name":"suite test","file":"example.test.ts"},{"name":"broken"}]`} {
		proc.result.Stdout = output
		if tests, err := mgr.Discover(context.Background()); err == nil || len(tests) != 1 || tests[0].Name != "known" {
			t.Fatalf("invalid or unsupported discovery %q: %v, %v", output, tests, err)
		}
	}
	reopened, err := manager.New(project, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 1 || snapshot.Tests[0].Name != "known" {
		t.Fatalf("persisted catalog: %v, %v", snapshot.Tests, err)
	}
}
