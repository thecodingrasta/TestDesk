package manager_test

import (
	"context"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

func TestDiscoveryPersistsTestsAfterLongOutputLine(t *testing.T) {
	project := t.TempDir()
	provider := domain.Provider{ID: "fixture", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{
		Parser: "lines", Command: domain.CommandSpec{Exe: "fixture"},
	}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	cfg := domain.ProjectConfig{Version: 1, Name: "long-line", Providers: []domain.Provider{provider}}
	proc := &discoveryFailureRunner{result: runner.Result{Stdout: "first\n#" + strings.Repeat("x", 70*1024) + "\nsecond\n"}}
	m, err := manager.New(project, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	tests, err := m.Discover(context.Background())
	if err != nil || len(tests) != 2 {
		t.Fatalf("discovery: %d tests, %v", len(tests), err)
	}
	reopened, err := manager.New(project, cfg, proc)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 2 {
		t.Fatalf("reopened catalog: %d tests, %v", len(snapshot.Tests), err)
	}
}
