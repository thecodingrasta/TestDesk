package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestRealJestProjectSpecificFileSelection(t *testing.T) {
	install := os.Getenv("TESTDESK_JEST_INSTALL")
	if install == "" {
		t.Skip("set TESTDESK_JEST_INSTALL to a disposable Jest installation")
	}
	root, err := os.MkdirTemp(install, "projects-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	t.Setenv("npm_config_cache", filepath.Join(root, ".npm-cache"))
	for name, content := range map[string]string{
		"package.json":   `{"private":true}`,
		"jest.config.js": `module.exports={maxWorkers:2,projects:[{displayName:'A',rootDir:__dirname,testMatch:['**/same.test.js'],globals:{marker:'A'}},{displayName:'B',rootDir:__dirname,testMatch:['**/same.test.js'],globals:{marker:'B'}}]};`,
		"same.test.js":   `test('same', () => require('fs').appendFileSync('marker',marker));`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	p := config.JestProvider()
	cfg := domain.ProjectConfig{Version: 1, Name: "jest", Providers: []domain.Provider{p}}
	m, err := manager.New(root, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 2 || cases[0].ID == cases[1].ID {
		t.Fatalf("project discovery: %+v, %v", cases, err)
	}
	plan, err := m.Plan(manager.RunRequest{ProviderIDs: []string{p.ID}})
	if err != nil || len(plan) != 1 || plan[0].TestID != "" {
		t.Fatalf("provider-wide plan: %+v, %v", plan, err)
	}
	for _, project := range []string{"A", "B"} {
		var selected domain.TestCase
		for _, tc := range cases {
			if tc.JestProject == project {
				selected = tc
			}
		}
		if selected.ID == "" {
			t.Fatalf("missing project %s: %+v", project, cases)
		}
		run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{selected.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if err := m.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := m.Diagnostics(run.ID, 4096)
		if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusPassed {
			t.Fatalf("project %s: %+v, %v, %s", project, diag.Run, err, diag.Log)
		}
		marker, err := os.ReadFile(filepath.Join(root, "marker"))
		if err != nil || string(marker) != project {
			t.Fatalf("project %s executions: %q, %v", project, marker, err)
		}
		if err := os.Remove(filepath.Join(root, "marker")); err != nil {
			t.Fatal(err)
		}
	}
	reopened, err := manager.New(root, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 2 {
		t.Fatalf("reopened catalog: %+v, %v", snapshot.Tests, err)
	}
	for _, tc := range snapshot.Tests {
		if tc.LastStatus != domain.StatusPassed {
			t.Fatalf("project result not persisted: %+v", tc)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "jest.config.js"), []byte(`module.exports={maxWorkers:2,projects:[{displayName:'A',rootDir:__dirname,testMatch:['**/same.test.js'],globals:{marker:'A'}},{displayName:'C',rootDir:__dirname,testMatch:['**/same.test.js'],globals:{marker:'C'}}]};`), 0600); err != nil {
		t.Fatal(err)
	}
	var stale string
	for _, tc := range cases {
		if tc.JestProject == "B" {
			stale = tc.ID
		}
	}
	run, err := reopened.Run(ctx, manager.RunRequest{TestIDs: []string{stale}})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := reopened.Diagnostics(run.ID, 4096)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed {
		t.Fatalf("stale project: %+v, %v", diag.Run, err)
	}
	if _, err := os.Stat(filepath.Join(root, "marker")); !os.IsNotExist(err) {
		t.Fatalf("stale project ran: %v", err)
	}
}
