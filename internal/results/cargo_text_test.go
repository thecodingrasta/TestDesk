package results

import (
	"github.com/thecodingrasta/testdesk/internal/domain"
	"testing"
)

func TestCargoTextCaseResultsAndZeroMatches(t *testing.T) {
	parsed, err := ParseCargoText("running 3 tests\ntest mod::pass ... ok\ntest mod::fail ... FAILED\ntest mod::skip ... ignored, needs network\n\ntest result: FAILED. 1 passed; 1 failed; 1 ignored\n")
	if err != nil || len(parsed) != 3 {
		t.Fatalf("results %+v: %v", parsed, err)
	}
	for i, want := range []domain.Status{domain.StatusPassed, domain.StatusFailed, domain.StatusSkipped} {
		if parsed[i].Status != want {
			t.Fatalf("result %d: %+v", i, parsed[i])
		}
	}
	parsed, err = ParseCargoText("running 0 tests\ntest result: ok. 0 passed; 0 failed; 0 ignored\n")
	if err != nil || len(parsed) != 0 {
		t.Fatalf("zero match: %+v, %v", parsed, err)
	}
}
