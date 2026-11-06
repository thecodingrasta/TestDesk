package manager_test

import (
	"context"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type duplicateCasesRunner struct{}

func (duplicateCasesRunner) Run(context.Context, domain.CommandSpec, func(runner.Event)) (runner.Result, error) {
	return runner.Result{Stdout: "The following Tests are available:\n  Fixture.Cases.Select(value: 1)\n  Fixture.Cases.Select(value: 1)\n"}, nil
}
func TestDuplicateDotNetCaseCannotBeTargeted(t *testing.T) {
	p := config.DotNetProvider()
	p.ID = "duplicates"
	p.Discover.Command = domain.CommandSpec{Exe: "fake"}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{p}}, duplicateCasesRunner{})
	if err != nil {
		t.Fatal(err)
	}
	cases, err := m.Discover(context.Background())
	if err != nil || len(cases) != 2 {
		t.Fatalf("cases: %+v %v", cases, err)
	}
	_, err = m.Run(context.Background(), manager.RunRequest{TestIDs: []string{cases[0].ID}})
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("expected ambiguity error, got %v", err)
	}
}
