package manager

import (
	"github.com/thecodingrasta/testdesk/internal/domain"
	"testing"
)

func TestPlanWarnsAboutPotentialDotNetOverlapWithoutMerging(t *testing.T) {
	p := []domain.Provider{{ID: "solution", Kind: domain.ProviderDotNet, Root: ".", Enabled: true, Run: domain.RunSpec{Test: domain.CommandSpec{Exe: "test"}}}, {ID: "project", Kind: domain.ProviderDotNet, Root: "tests/unit", Enabled: true, Run: domain.RunSpec{Test: domain.CommandSpec{Exe: "test"}}}}
	m := &Manager{cfg: domain.ProjectConfig{Providers: p}, tests: map[string]domain.TestCase{
		"a": {ID: "a", ProviderID: "solution", ProviderKind: domain.ProviderDotNet, FullName: "Same.Namespace.Test"},
		"b": {ID: "b", ProviderID: "project", ProviderKind: domain.ProviderDotNet, FullName: "Same.Namespace.Test"},
		"c": {ID: "c", ProviderID: "project", ProviderKind: domain.ProviderDotNet, FullName: "Other.Test"},
	}}
	items, err := m.Plan(RunRequest{TestIDs: []string{"a", "b", "c"}})
	if err != nil || len(items) != 3 {
		t.Fatalf("plan: %+v %v", items, err)
	}
	if len(items[0].Warnings) == 0 || len(items[1].Warnings) == 0 || len(items[2].Warnings) != 0 {
		t.Fatalf("warnings: %+v", items)
	}
	single, err := m.Plan(RunRequest{TestIDs: []string{"a"}})
	if err != nil || len(single) != 1 || len(single[0].Warnings) != 0 {
		t.Fatalf("single provider: %+v %v", single, err)
	}
}
func TestDotNetRootOverlap(t *testing.T) {
	if !dotNetRootsMayOverlap(".", "tests/unit") || dotNetRootsMayOverlap("tests/one", "tests/two") {
		t.Fatal("scope overlap classification incorrect")
	}
}

func TestProviderWidePlanWarnsAboutOverlappingDotNetRoots(t *testing.T) {
	providers := []domain.Provider{
		{ID: "solution", Kind: domain.ProviderDotNet, Root: ".", Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "test"}}},
		{ID: "project", Kind: domain.ProviderDotNet, Root: "tests/unit", Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "test"}}},
	}
	m, err := New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: providers}, nil)
	if err != nil {
		t.Fatal(err)
	}
	items, err := m.Plan(RunRequest{ProviderIDs: []string{"solution", "project"}})
	if err != nil || len(items) != 2 || len(items[0].Warnings) == 0 || len(items[1].Warnings) == 0 {
		t.Fatalf("provider-wide plan: %+v %v", items, err)
	}
}
