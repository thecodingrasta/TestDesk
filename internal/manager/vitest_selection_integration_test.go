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

func TestRealVitestExactSelectionFailureAndRecovery(t *testing.T) {
	fixture := os.Getenv("TESTDESK_VITEST_FIXTURE")
	if fixture == "" {
		t.Skip("requires a disposable Vitest fixture")
	}
	root, err := os.MkdirTemp(fixture, "selection-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	file := filepath.Join(root, "selection.test.js")
	marker := filepath.Join(root, "marker.txt")
	t.Setenv("MARKER", marker)
	write := func(body string) {
		t.Helper()
		if err := os.WriteFile(file, []byte("import { test, describe } from 'vitest';\nimport { appendFileSync, existsSync, writeFileSync } from 'node:fs';\n"+body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(`describe('suite', () => { test('target [x]+', () => appendFileSync(process.env.MARKER, 'target\n')); });`)
	if err := os.WriteFile(filepath.Join(root, "vitest.config.mjs"), []byte("import { defineConfig } from 'vitest/config'; export default defineConfig({ test: {} });\n"), 0600); err != nil {
		t.Fatal(err)
	}
	provider := config.VitestProvider()
	provider.Discover.Command = domain.CommandSpec{Exe: "node", Args: []string{filepath.Join(fixture, "node_modules", "vitest", "vitest.mjs"), "list", "--json"}, WorkingDir: root, TimeoutSeconds: 60}
	provider.Run.Test.WorkingDir = root
	mgr, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{provider}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	tests, err := mgr.Discover(ctx)
	if err != nil || len(tests) != 1 {
		t.Fatalf("discovery: %+v, %v", tests, err)
	}
	run := func(expected, expectedCase domain.Status, wantMarker bool) {
		t.Helper()
		_ = os.Remove(marker)
		queued, err := mgr.Run(ctx, manager.RunRequest{TestIDs: []string{tests[0].ID}})
		if err != nil {
			t.Fatal(err)
		}
		if err := mgr.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := mgr.Diagnostics(queued.ID, 4096)
		if err != nil || diag.Run.Status != expected {
			t.Fatalf("run expected %s: %+v, %v", expected, diag.Run, err)
		}
		snapshot, err := mgr.Snapshot(domain.Filter{Limit: 10})
		if err != nil || len(snapshot.Tests) != 1 || snapshot.Tests[0].LastStatus != expectedCase {
			t.Fatalf("case expected %s: %+v, %v", expectedCase, snapshot.Tests, err)
		}
		_, markerErr := os.Stat(marker)
		if (markerErr == nil) != wantMarker {
			t.Fatalf("unexpected test body execution: %v", markerErr)
		}
	}
	run(domain.StatusPassed, domain.StatusPassed, true)
	write(`describe('suite', () => { test('target [x]+', () => { throw Error('expected'); }); });`)
	run(domain.StatusFailed, domain.StatusFailed, false)
	write(`describe('suite', () => { test.skip('target [x]+', () => appendFileSync(process.env.MARKER, 'wrong')); });`)
	run(domain.StatusSkipped, domain.StatusSkipped, false)
	write(`describe('suite', () => { test('renamed', () => appendFileSync(process.env.MARKER, 'wrong')); });`)
	run(domain.StatusFailed, domain.StatusUnknown, false)
	write(`describe('suite', () => { test('target [x]+', () => appendFileSync(process.env.MARKER, 'wrong')); test('target [x]+', () => appendFileSync(process.env.MARKER, 'wrong')); });`)
	run(domain.StatusFailed, domain.StatusUnknown, false)
	write(`const counter = 'counter.txt'; const first = !existsSync(counter); writeFileSync(counter, '1'); describe('suite', () => { test(first ? 'target [x]+' : 'wrong', () => appendFileSync(process.env.MARKER, 'wrong')); });`)
	_ = os.Remove(filepath.Join(root, "counter.txt"))
	run(domain.StatusFailed, domain.StatusUnknown, false)
}
