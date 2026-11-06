package discovery

import (
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestTextDiscoveryContinuesAfterLongOutputLine(t *testing.T) {
	longNoise := "#" + strings.Repeat("x", 70*1024)
	for _, tc := range []struct{ parser, first, second string }{
		{"lines", "first", "second"},
		{"dotnet-list", "  Fixture.Cases.First", "  Fixture.Cases.Second"},
		{"playwright-list", "first.spec.ts:1 › first", "second.spec.ts:2 › second"},
		{"go-list", "TestFirst", "TestSecond"},
		{"pytest-collect", "test_first.py::test_first", "test_second.py::test_second"},
		{"cargo-list", "first: test", "second: test"},
	} {
		t.Run(tc.parser, func(t *testing.T) {
			provider := domain.Provider{ID: "fixture", Discover: domain.DiscoverSpec{Parser: tc.parser}}
			tests, err := Parse(provider, tc.first+"\n"+longNoise+"\n"+tc.second+"\n")
			if err != nil || len(tests) != 2 {
				t.Fatalf("discovery stopped at long line: got %d tests, err %v", len(tests), err)
			}
		})
	}
}

func TestTextDiscoveryAcceptsLongTestIdentifierAtEndOfOutput(t *testing.T) {
	provider := domain.Provider{ID: "fixture", Discover: domain.DiscoverSpec{Parser: "lines"}}
	longName := "test-" + strings.Repeat("x", 70*1024)
	tests, err := Parse(provider, longName)
	if err != nil || len(tests) != 1 || tests[0].Name != longName {
		t.Fatalf("long final identifier was not preserved: got %d tests, err %v", len(tests), err)
	}
}
