package discovery_test

import (
	"testing"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/discovery"
)

func TestVitestFlatDiscoveryPreservesProjectAndFileIdentity(t *testing.T) {
	provider := config.VitestProvider()
	output := `[{"name":"suite case [a]+","file":"/work/a.test.js","projectName":"alpha","location":{"line":5}},{"name":"suite case [a]+","file":"/work/a.test.js","projectName":"beta"},{"name":"suite case [a]+","file":"/work/b.test.js","projectName":"alpha"}]`
	tests, err := discovery.Parse(provider, output)
	if err != nil || len(tests) != 3 {
		t.Fatalf("flat discovery: %+v, %v", tests, err)
	}
	seen := make(map[string]bool)
	projects := make(map[string]int)
	for _, test := range tests {
		if seen[test.ID] {
			t.Fatalf("duplicate identity: %+v", tests)
		}
		seen[test.ID] = true
		projects[test.VitestProject]++
	}
	if projects["alpha"] != 2 || projects["beta"] != 1 {
		t.Fatalf("project identities: %+v", tests)
	}
	duplicate := `[{"name":"same","file":"/work/a.test.js"},{"name":"same","file":"/work/a.test.js"}]`
	if tests, err := discovery.Parse(provider, duplicate); err == nil {
		t.Fatalf("ambiguous duplicate accepted: %+v", tests)
	}
}
