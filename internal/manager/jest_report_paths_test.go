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
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type jestReportWriter struct{ paths chan string }

func (r jestReportWriter) Run(_ context.Context, cmd domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	path := cmd.Env["JEST_JUNIT_OUTPUT_FILE"]
	if err := os.WriteFile(path, []byte(`<testsuites><testsuite name="new"><testcase classname="new" name="passing"/></testsuite></testsuites>`), 0600); err != nil {
		return runner.Result{}, err
	}
	r.paths <- path
	return runner.Result{}, nil
}

func TestJestRunsUseDistinctReportPaths(t *testing.T) {
	root := t.TempDir()
	paths := make(chan string, 2)
	provider := config.JestProvider()
	m, err := manager.New(root, domain.ProjectConfig{Version: 1, Name: "jest", Providers: []domain.Provider{provider}}, jestReportWriter{paths: paths})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var outputPaths []string
	for i := 0; i < 2; i++ {
		run, err := m.Run(ctx, manager.RunRequest{ProviderIDs: []string{provider.ID}})
		if err != nil {
			t.Fatal(err)
		}
		if err := m.WaitForIdle(ctx); err != nil {
			t.Fatal(err)
		}
		diag, err := m.Diagnostics(run.ID, 1024)
		if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusPassed || diag.Run.Passed != 1 {
			t.Fatalf("run: %+v, %v", diag.Run, err)
		}
		select {
		case path := <-paths:
			outputPaths = append(outputPaths, path)
		default:
			t.Fatal("report was not written")
		}
	}
	if outputPaths[0] == outputPaths[1] || !filepath.IsAbs(outputPaths[0]) || !filepath.IsAbs(outputPaths[1]) {
		t.Fatalf("invalid report paths: %q", outputPaths)
	}
}
