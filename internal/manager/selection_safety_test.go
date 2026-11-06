package manager

import (
	"context"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestTargetedSelectionNeverWidens(t *testing.T) {
	provider := domain.Provider{ID: "unit", Name: "Unit", Enabled: true, Run: domain.RunSpec{
		All: domain.CommandSpec{Exe: "all"}, Test: domain.CommandSpec{Exe: "one"},
	}}
	m := &Manager{cfg: domain.ProjectConfig{Providers: []domain.Provider{provider}}, tests: map[string]domain.TestCase{
		"valid": {ID: "valid", ProviderID: "unit", Name: "Known"},
	}}
	cases := []struct {
		name    string
		request RunRequest
	}{
		{"unknown ID", RunRequest{TestIDs: []string{"nonexistent-test-id"}}},
		{"stale ID", RunRequest{TestIDs: []string{"previous-catalog-id"}}},
		{"mixed IDs", RunRequest{TestIDs: []string{"valid", "missing"}}},
		{"unmatched query", RunRequest{Filter: domain.Filter{Query: "does-not-exist"}}},
		{"unmatched filter", RunRequest{Filter: domain.Filter{Layers: []string{"missing"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if items, err := m.Plan(tc.request); err == nil || len(items) != 0 {
				t.Fatalf("plan widened selection: items=%+v err=%v", items, err)
			}
			if run, err := m.Run(context.Background(), tc.request); err == nil || run.ID != "" {
				t.Fatalf("run accepted selection: run=%+v err=%v", run, err)
			}
		})
	}
	items, err := m.Plan(RunRequest{TestIDs: []string{"valid"}})
	if err != nil || len(items) != 1 || items[0].TestID != "valid" || items[0].CommandPreview != "one" {
		t.Fatalf("valid targeted plan: items=%+v err=%v", items, err)
	}
	provider.Run.Test = domain.CommandSpec{}
	m.cfg.Providers[0] = provider
	if items, err := m.Plan(RunRequest{TestIDs: []string{"valid"}}); err == nil || len(items) != 0 {
		t.Fatalf("plan used all command for selected test: items=%+v err=%v", items, err)
	}
	if _, err := m.Run(context.Background(), RunRequest{TestIDs: []string{"valid"}}); err == nil {
		t.Fatal("run accepted provider without targeted command")
	}
	status, message := m.executeItem(context.Background(), domain.RunItem{ProviderID: "unit", TestID: "valid"})
	if status != domain.StatusFailed || !strings.Contains(message, "targeted") {
		t.Fatalf("execution used all command: status=%s message=%q", status, message)
	}
	status, message = m.executeItem(context.Background(), domain.RunItem{ProviderID: "unit", TestID: "stale"})
	if status != domain.StatusFailed || !strings.Contains(message, "catalog") {
		t.Fatalf("execution accepted stale test: status=%s message=%q", status, message)
	}
	m.tests = nil
	items, err = m.Plan(RunRequest{ProviderIDs: []string{"unit"}})
	if err != nil || len(items) != 1 || items[0].TestID != "" || items[0].CommandPreview != "all" {
		t.Fatalf("intentional provider-wide plan: items=%+v err=%v", items, err)
	}
}
