package mcp_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/mcp"
)

func TestMCPDiagnosticsRejectsRunPathTraversal(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(mgr.StoreRoot(), "sibling")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatal(err)
	}
	const sentinel = "private sibling log"
	if err := os.WriteFile(filepath.Join(outside, "output.log"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	request := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_diagnostics","arguments":{"runId":"../sibling","maxBytes":4096}}}` + "\n"
	var output bytes.Buffer
	if err := mcp.New(mgr).Serve(context.Background(), strings.NewReader(request), &output); err != nil {
		t.Fatal(err)
	}
	response := output.String()
	if !strings.Contains(response, `"error"`) || !strings.Contains(response, "invalid run ID") || strings.Contains(response, sentinel) {
		t.Fatalf("unexpected diagnostics response: %s", response)
	}
}
