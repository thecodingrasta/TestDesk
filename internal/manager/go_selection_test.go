package manager

import (
	"context"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestStaleGoSelectionFailsBeforeQueue(t *testing.T) {
	provider := config.GoTestProvider()
	m := &Manager{cfg: domain.ProjectConfig{Providers: []domain.Provider{provider}}, tests: map[string]domain.TestCase{
		"old": {ID: "old", ProviderID: provider.ID, ProviderKind: domain.ProviderGo, Name: "TestShared"},
	}}
	request := RunRequest{TestIDs: []string{"old"}}
	if items, err := m.Plan(request); err == nil || len(items) != 0 {
		t.Fatalf("plan accepted stale selection: %+v %v", items, err)
	}
	if run, err := m.Run(context.Background(), request); err == nil || run.ID != "" {
		t.Fatalf("run accepted stale selection: %+v %v", run, err)
	}
}

func TestGoResultIdentityIncludesPackage(t *testing.T) {
	test := domain.TestCase{ProviderKind: domain.ProviderGo, GoPackage: "example/one", Name: "TestShared", FullName: "example/one.TestShared", Selector: "TestShared"}
	other := domain.TestResult{Name: "TestShared", FullName: "example/two.TestShared", Selector: "TestShared", Status: domain.StatusPassed}
	if matched, _ := selectedResultStatus(test, []domain.TestResult{other}); matched {
		t.Fatal("result from a different Go package matched selected test")
	}
	m := &Manager{tests: map[string]domain.TestCase{"one": test}}
	test.ID, test.ProviderID = "one", "go"
	m.tests["one"] = test
	if got := m.matchTestCandidatesLocked("go", other); len(got) != 0 {
		t.Fatalf("wrong package attributed to %v", got)
	}
	other.FullName = "example/one.TestShared"
	if got := m.matchTestCandidatesLocked("go", other); len(got) != 1 || got[0] != "one" {
		t.Fatalf("matching package not attributed: %v", got)
	}
}

func TestGoBenchmarkSelectionUsesBenchCommand(t *testing.T) {
	provider := config.GoTestProvider()
	m := &Manager{cfg: domain.ProjectConfig{Providers: []domain.Provider{provider}}, tests: map[string]domain.TestCase{
		"bench": {ID: "bench", ProviderID: provider.ID, ProviderKind: domain.ProviderGo, Name: "BenchmarkWork", GoPackage: "example/bench"},
	}}
	m.tests["bench"] = domain.TestCase{ID: "bench", ProviderID: provider.ID, ProviderKind: domain.ProviderGo, Name: "BenchmarkWork", Selector: "BenchmarkWork", GoPackage: "example/bench"}
	items, err := m.Plan(RunRequest{TestIDs: []string{"bench"}})
	if err != nil || len(items) != 1 || !strings.Contains(items[0].CommandPreview, "-bench ^BenchmarkWork$") {
		t.Fatalf("benchmark plan: %+v %v", items, err)
	}
}
