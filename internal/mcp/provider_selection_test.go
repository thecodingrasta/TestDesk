package mcp

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestProviderOnlyRequestPlansUndiscoveredProvider(t *testing.T) {
	p := domain.Provider{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{p}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	request := runRequestFromArgs(map[string]any{"providerIds": []any{"fixture"}})
	items, err := m.Plan(request)
	if err != nil || len(items) != 1 || items[0].ProviderID != "fixture" || items[0].TestID != "" {
		t.Fatalf("provider-wide plan: %+v %v", items, err)
	}
	request = runRequestFromArgs(map[string]any{"providerIds": []any{"fixture"}, "query": "missing"})
	if items, err = m.Plan(request); err == nil || len(items) != 0 {
		t.Fatalf("unmatched query widened: %+v %v", items, err)
	}
}
