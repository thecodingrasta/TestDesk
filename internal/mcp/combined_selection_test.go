package mcp

import (
	"context"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
	"testing"
)

type selectionConstraintRunner struct{}

func (selectionConstraintRunner) Run(_ context.Context, command domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	return runner.Result{Stdout: command.Exe + "\n"}, nil
}

func TestMCPCombinedSelectionCannotIgnoreFilters(t *testing.T) {
	providers := []domain.Provider{}
	for _, name := range []string{"alpha", "beta"} {
		providers = append(providers, domain.Provider{ID: name, Name: name, Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: name}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all"}, Test: domain.CommandSpec{Exe: "one"}}})
	}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: providers}, selectionConstraintRunner{})
	if err != nil {
		t.Fatal(err)
	}
	cases, err := m.Discover(context.Background())
	if err != nil || len(cases) != 2 {
		t.Fatalf("discovery: %+v %v", cases, err)
	}
	var alpha string
	for _, tc := range cases {
		if tc.ProviderID == "alpha" {
			alpha = tc.ID
		}
	}
	for _, args := range []map[string]any{
		{"testIds": []any{alpha}, "providerIds": []any{"beta"}},
		{"testIds": []any{alpha}, "query": "beta"},
		{"providerIds": []any{"alpha"}, "query": "beta"},
	} {
		if items, err := m.Plan(runRequestFromArgs(args)); err == nil || len(items) != 0 {
			t.Fatalf("MCP constraints widened: %+v %v", items, err)
		}
	}
}
