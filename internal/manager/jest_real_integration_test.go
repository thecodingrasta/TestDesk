package manager_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestRealJestExactFileAndReport(t *testing.T) {
	install := os.Getenv("TESTDESK_JEST_INSTALL")
	if install == "" {
		t.Skip("set TESTDESK_JEST_INSTALL to a disposable Jest installation")
	}
	root, err := os.MkdirTemp(install, "fixture-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	t.Setenv("npm_config_cache", filepath.Join(root, ".npm-cache"))
	for name, content := range map[string]string{
		"package.json":            `{"private":true,"jest":{"maxWorkers":2}}`,
		"a.test.js":               `test('first', () => require('fs').appendFileSync('marker', 'A'));`,
		"a.test.js.extra.test.js": `test('second', () => require('fs').appendFileSync('marker', 'B'));`,
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	provider := config.JestProvider()
	cfg := domain.ProjectConfig{Version: 1, Name: "jest", Providers: []domain.Provider{provider}}
	m, err := manager.New(root, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tests, err := m.Discover(ctx)
	if err != nil || len(tests) != 2 {
		t.Fatalf("discovery: %+v, %v", tests, err)
	}
	var selected domain.TestCase
	for _, tc := range tests {
		if strings.HasSuffix(tc.Selector, string(filepath.Separator)+"a.test.js") {
			selected = tc
		}
	}
	if selected.ID == "" {
		t.Fatalf("missing selected file: %+v", tests)
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
		t.Fatalf("run: %+v, %v, log: %s", diag.Run, err, diag.Log)
	}
	marker, err := os.ReadFile(filepath.Join(root, "marker"))
	if err != nil || string(marker) != "A" {
		t.Fatalf("test execution: %q, %v", marker, err)
	}
	reopened, err := manager.New(root, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	assertStatus := func(want domain.Status) {
		t.Helper()
		current, err := manager.New(root, cfg, nil)
		if err != nil {
			t.Fatal(err)
		}
		state, err := current.Snapshot(domain.Filter{Limit: 10})
		if err != nil {
			t.Fatal(err)
		}
		for _, tc := range state.Tests {
			if tc.ID == selected.ID && tc.LastStatus == want {
				return
			}
		}
		t.Fatalf("selected status not %s: %+v", want, state.Tests)
	}
	assertStatus(domain.StatusPassed)
	if err := os.WriteFile(filepath.Join(root, "a.test.js"), []byte(`test('first', () => { throw Error('fixture failure'); });`), 0600); err != nil {
		t.Fatal(err)
	}
	failed, err := reopened.Run(ctx, manager.RunRequest{TestIDs: []string{selected.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	failedDiag, err := reopened.Diagnostics(failed.ID, 4096)
	if err != nil || failedDiag.Run == nil || failedDiag.Run.Status != domain.StatusFailed {
		t.Fatalf("failed test: %+v, %v", failedDiag.Run, err)
	}
	assertStatus(domain.StatusFailed)
	if err := os.Remove(filepath.Join(root, "a.test.js")); err != nil {
		t.Fatal(err)
	}
	missing, err := reopened.Run(ctx, manager.RunRequest{TestIDs: []string{selected.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := reopened.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	missingDiag, err := reopened.Diagnostics(missing.ID, 4096)
	if err != nil || missingDiag.Run == nil || missingDiag.Run.Status != domain.StatusFailed {
		t.Fatalf("missing file: %+v, %v", missingDiag.Run, err)
	}
	assertStatus(domain.StatusUnknown)
	marker, err = os.ReadFile(filepath.Join(root, "marker"))
	if err != nil || string(marker) != "A" {
		t.Fatalf("unselected file executed: %q, %v", marker, err)
	}
}
