package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestWaitOmitsLargeCatalog(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	project := t.TempDir()
	config := `{"version":1,"name":"compact","workspace":".testdesk","providers":[{"id":"fixture","name":"fixture","kind":"custom","enabled":true,"run":{"all":{"exe":"go","args":["version"]}}}]}`
	if err := os.WriteFile(filepath.Join(project, "testdesk.json"), []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(project, ".testdesk"), 0o700); err != nil {
		t.Fatal(err)
	}
	tests := make([]domain.TestCase, 10000)
	for i := range tests {
		tests[i] = domain.TestCase{ID: fmt.Sprint(i), ProviderID: "fixture", Name: fmt.Sprintf("case-%d", i)}
	}
	data, err := json.Marshal(tests)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".testdesk", "catalog.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	server := New(mgr)
	check := func(wantIdle bool) {
		t.Helper()
		payload, err := server.wait(context.Background(), map[string]any{"timeoutMs": 10})
		if err != nil {
			t.Fatal(err)
		}
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		result := payload.(map[string]any)
		if result["idle"] != wantIdle {
			t.Fatalf("idle = %v, want %v", result["idle"], wantIdle)
		}
		if _, ok := result["testPage"]; ok || len(body) > 8192 {
			t.Fatalf("wait returned catalog or oversized response (%d bytes)", len(body))
		}
	}
	check(true)
	var wire bytes.Buffer
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_wait","arguments":{"timeoutMs":10}}}` + "\n"
	if err := server.Serve(context.Background(), strings.NewReader(request), &wire); err != nil {
		t.Fatal(err)
	}
	if wire.Len() > 8192 || strings.Contains(wire.String(), "testPage") || strings.Contains(wire.String(), "case-9999") {
		t.Fatalf("wire wait included catalog or exceeded response bound (%d bytes)", wire.Len())
	}
	if err := mgr.Control(manager.ControlPause); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Run(context.Background(), manager.RunRequest{}); err != nil {
		t.Fatal(err)
	}
	check(false)
	if err := mgr.Control(manager.ControlResume); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
}
