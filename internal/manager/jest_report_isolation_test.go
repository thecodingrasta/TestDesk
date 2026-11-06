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

type jestNoReportRunner struct{}

func (jestNoReportRunner) Run(context.Context, domain.CommandSpec, func(runner.Event)) (runner.Result, error) {
	return runner.Result{}, nil
}

func TestJestRunDoesNotAcceptOldProjectReport(t *testing.T) {
	root := t.TempDir()
	oldReport := []byte(`<testsuites><testsuite name="old"><testcase classname="old" name="passing"/></testsuite></testsuites>`)
	if err := os.WriteFile(filepath.Join(root, "junit.xml"), oldReport, 0600); err != nil {
		t.Fatal(err)
	}
	provider := config.JestProvider()
	m, err := manager.New(root, domain.ProjectConfig{Version: 1, Name: "jest", Providers: []domain.Provider{provider}}, jestNoReportRunner{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := m.Run(context.Background(), manager.RunRequest{ProviderIDs: []string{provider.ID}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	diag, err := m.Diagnostics(run.ID, 1024)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed || diag.Run.Passed != 0 {
		t.Fatalf("stale report accepted: %+v, %v", diag.Run, err)
	}
	actual, err := os.ReadFile(filepath.Join(root, "junit.xml"))
	if err != nil || string(actual) != string(oldReport) {
		t.Fatalf("existing report changed: %q, %v", actual, err)
	}
}
