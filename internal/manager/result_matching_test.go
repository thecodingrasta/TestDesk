package manager

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestReportMatchingRequiresUniqueIdentity(t *testing.T) {
	m := &Manager{tests: map[string]domain.TestCase{
		"first":  {ID: "first", ProviderID: "p", Name: "Same", FullName: "Fixture.First.Same", Selector: "Fixture.First.Same"},
		"second": {ID: "second", ProviderID: "p", Name: "Same", FullName: "Fixture.Second.Same", Selector: "Fixture.Second.Same"},
	}}
	if got := m.matchTestCandidatesLocked("p", domain.TestResult{Name: "Same", FullName: "Fixture.Second.Same"}); len(got) != 1 || got[0] != "second" {
		t.Fatalf("exact name matched %q", got)
	}
	if got := m.matchTestCandidatesLocked("p", domain.TestResult{Name: "Same"}); len(got) != 2 {
		t.Fatalf("ambiguous short name candidates %q", got)
	}
	m.tests["first"] = domain.TestCase{ID: "first", ProviderID: "p", Name: "Same", FullName: "Fixture.First.Same", Selector: "Same"}
	m.tests["second"] = domain.TestCase{ID: "second", ProviderID: "p", Name: "Same", FullName: "Fixture.Second.Same", Selector: "Same"}
	if got := m.matchTestCandidatesLocked("p", domain.TestResult{Name: "Same", Selector: "Same", FullName: "Fixture.Second.Same"}); len(got) != 1 || got[0] != "second" {
		t.Fatalf("full identity should take precedence over shared selector: %q", got)
	}
	m.tests["third"] = domain.TestCase{ID: "third", ProviderID: "p", Name: "Same", FullName: "Fixture.Second.Same", Selector: "Fixture.Second.Same"}
	if got := m.matchTestCandidatesLocked("p", domain.TestResult{Name: "Same", FullName: "Fixture.Second.Same"}); len(got) != 2 {
		t.Fatalf("ambiguous full name candidates %q", got)
	}
}
