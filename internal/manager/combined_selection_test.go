package manager

import (
	"context"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"testing"
)

func TestExplicitIDsMustSatisfySelectionConstraints(t *testing.T) {
	providers := []domain.Provider{
		{ID: "one", Enabled: true, Run: domain.RunSpec{Test: domain.CommandSpec{Exe: "single"}}},
		{ID: "two", Enabled: true, Run: domain.RunSpec{Test: domain.CommandSpec{Exe: "single"}}},
	}
	m := &Manager{cfg: domain.ProjectConfig{Providers: providers}, tests: map[string]domain.TestCase{
		"a": {ID: "a", ProviderID: "one", Name: "Alpha", Layer: "unit"},
		"b": {ID: "b", ProviderID: "two", Name: "Beta", Layer: "integration"},
	}}
	invalid := []RunRequest{
		{TestIDs: []string{"a"}, ProviderIDs: []string{"two"}},
		{TestIDs: []string{"a"}, Filter: domain.Filter{Query: "not-alpha"}},
		{TestIDs: []string{"a", "b"}, Filter: domain.Filter{Layers: []string{"unit"}}},
	}
	for _, request := range invalid {
		if items, err := m.Plan(request); err == nil || len(items) != 0 {
			t.Fatalf("contradictory plan accepted: %+v %v", items, err)
		}
		if run, err := m.Run(context.Background(), request); err == nil || run.ID != "" {
			t.Fatalf("contradictory run accepted: %+v %v", run, err)
		}
	}
	items, err := m.Plan(RunRequest{TestIDs: []string{"a"}, ProviderIDs: []string{"one"}, Filter: domain.Filter{Query: "alpha"}})
	if err != nil || len(items) != 1 || items[0].TestID != "a" {
		t.Fatalf("valid combined selection: %+v %v", items, err)
	}
	items, err = m.Plan(RunRequest{ProviderIDs: []string{"one"}, Filter: domain.Filter{Query: "beta"}})
	if err == nil || len(items) != 0 {
		t.Fatalf("provider and query widened: %+v %v", items, err)
	}
	items, err = m.Plan(RunRequest{ProviderIDs: []string{"two"}, Filter: domain.Filter{Query: "a", Limit: 1}})
	if err != nil || len(items) != 1 || items[0].TestID != "b" {
		t.Fatalf("provider constraint must precede pagination: %+v %v", items, err)
	}
}

func TestRerunProviderWideScopeFailsClosed(t *testing.T) {
	providers := []domain.Provider{
		{ID: "one", Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-one"}, Test: domain.CommandSpec{Exe: "one"}}},
		{ID: "two", Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all-two"}}},
	}
	m := &Manager{cfg: domain.ProjectConfig{Providers: providers}, tests: map[string]domain.TestCase{"a": {ID: "a", ProviderID: "one", Name: "Alpha"}}}
	items, err := m.Plan(RunRequest{ProviderWideIDs: []string{"two", "two"}})
	if err != nil || len(items) != 1 || items[0].ProviderID != "two" || items[0].TestID != "" {
		t.Fatalf("provider-only rerun scope: %+v %v", items, err)
	}
	for _, request := range []RunRequest{
		{TestIDs: []string{"a"}, ProviderWideIDs: []string{"one"}},
		{ProviderWideIDs: []string{"missing"}},
		{ProviderWideIDs: []string{"two"}, Filter: domain.Filter{Query: "Alpha"}},
	} {
		if items, err := m.Plan(request); err == nil || len(items) != 0 {
			t.Fatalf("unsafe rerun plan: %+v %v", items, err)
		}
	}
}
