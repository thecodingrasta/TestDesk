package discovery_test

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/discovery"
)

func TestVitestJSONRejectsIncompleteOutput(t *testing.T) {
	provider := config.VitestProvider()
	for _, output := range []string{"", `[{"name":"partial"`, "discovery failed\n", `[{"name":"suite test","file":"example.test.ts"},{"name":"broken"}]`, `null`, `{}`} {
		if tests, err := discovery.Parse(provider, output); err == nil {
			t.Errorf("output %q accepted as %v", output, tests)
		}
	}
	if tests, err := discovery.Parse(provider, `[]`); err != nil || len(tests) != 0 {
		t.Fatalf("valid empty catalog: %v, %v", tests, err)
	}
}
