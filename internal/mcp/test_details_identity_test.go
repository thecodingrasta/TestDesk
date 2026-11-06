package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestDetailsRequireUnambiguousIdentityAndOwnFailure(t *testing.T) {
	providers := []domain.Provider{}
	for _, id := range []string{"first", "second"} {
		providers = append(providers, domain.Provider{ID: id, Name: id, Enabled: true, Kind: domain.ProviderCustom, Discover: domain.DiscoverSpec{Parser: "lines", Command: domain.CommandSpec{Exe: "shared"}}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "all"}}})
	}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: providers}, selectionConstraintRunner{})
	if err != nil {
		t.Fatal(err)
	}
	tests, err := m.Discover(context.Background())
	if err != nil || len(tests) != 2 {
		t.Fatalf("discovery: %+v, %v", tests, err)
	}
	s := New(m)
	for _, args := range []map[string]any{
		{"testName": "shared"},
		{"testId": "missing", "testName": "shared"},
		{"testId": tests[0].ID, "testName": "different"},
	} {
		if result, err := s.testDetails(args); err == nil {
			t.Fatalf("accepted conflicting identity %v: %+v", args, result)
		}
	}
	other := domain.Run{ID: "fixture-run", Status: domain.StatusFailed, StartedAt: time.Now().UTC(), Items: []domain.RunItem{{TestID: tests[1].ID, TestName: tests[0].Name, Status: domain.StatusFailed, Error: "other test failure"}}}
	if err := m.Store().AppendHistory(other); err != nil {
		t.Fatal(err)
	}
	value, err := s.testDetails(map[string]any{"testId": tests[0].ID})
	if err != nil {
		t.Fatal(err)
	}
	details := value.(map[string]any)
	if details["id"] != tests[0].ID || details["lastFailure"] != nil {
		t.Fatalf("incorrect details: %+v", details)
	}
	own := domain.Run{ID: "own-run", Status: domain.StatusFailed, StartedAt: time.Now().UTC(), Items: []domain.RunItem{{TestID: tests[0].ID, TestName: tests[0].Name, Status: domain.StatusFailed, Error: "own failure"}}}
	if err := m.Store().AppendHistory(own); err != nil {
		t.Fatal(err)
	}
	value, err = s.testDetails(map[string]any{"testId": tests[0].ID, "testName": "shared"})
	if err != nil {
		t.Fatal(err)
	}
	details = value.(map[string]any)
	failure, ok := details["lastFailure"].(map[string]any)
	if !ok || failure["error"] != "own failure" {
		t.Fatalf("own failure missing: %+v", details)
	}
	requests := fmt.Sprintf("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"tools/call\",\"params\":{\"name\":\"testdesk_test_details\",\"arguments\":{\"testName\":\"shared\"}}}\n{\"jsonrpc\":\"2.0\",\"id\":2,\"method\":\"tools/call\",\"params\":{\"name\":\"testdesk_test_details\",\"arguments\":{\"testId\":%q}}}\n", tests[0].ID)
	var output bytes.Buffer
	if err := s.Serve(context.Background(), strings.NewReader(requests), &output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("unexpected responses: %s", output.String())
	}
	var first, second map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatal(err)
	}
	if len(first["error"]) == 0 || !strings.Contains(string(first["error"]), "ambiguous") || len(second["result"]) == 0 || !strings.Contains(string(second["result"]), "own failure") {
		t.Fatalf("unexpected protocol responses: %s", output.String())
	}
}
