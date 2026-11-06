package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thecodingrasta/testdesk/internal/mcp"
	"github.com/thecodingrasta/testdesk/internal/prefs"
)

func TestProjectPagesAndExplicitMissingRemoval(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	parent := t.TempDir()
	existing := t.TempDir()
	refs := []prefs.ProjectRef{{Root: existing, Name: "existing"}}
	for i := 0; i < 125; i++ {
		refs = append(refs, prefs.ProjectRef{Root: filepath.Join(parent, fmt.Sprintf("missing-%03d", i)), Name: fmt.Sprintf("missing-%03d", i)})
	}
	if err := prefs.Save(prefs.Preferences{Projects: refs}); err != nil {
		t.Fatal(err)
	}
	server := mcp.New(nil)
	first := projectCall(t, server, "testdesk_projects", nil)
	if int(first["total"].(float64)) != 126 || len(first["projects"].([]any)) != 50 || first["hasMore"] != true {
		t.Fatalf("default page: %+v", first)
	}
	last := projectCall(t, server, "testdesk_projects", map[string]any{"offset": 100, "limit": 1000})
	if len(last["projects"].([]any)) != 26 || last["limit"] != float64(100) || last["hasMore"] != false {
		t.Fatalf("last page: %+v", last)
	}
	missing := refs[1].Root
	projectCall(t, server, "testdesk_project_forget_missing", map[string]any{"projectRoot": missing})
	after := projectCall(t, server, "testdesk_projects", map[string]any{"limit": 100})
	if after["total"] != float64(125) {
		t.Fatalf("removal changed wrong number of projects: %+v", after)
	}
	value, err := prefs.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, project := range value.Projects {
		if project.Root == missing {
			t.Fatalf("missing root was not forgotten: %s", missing)
		}
	}
	projectCallError(t, server, "testdesk_project_forget_missing", map[string]any{"projectRoot": existing})
	projectCallError(t, server, "testdesk_project_forget_missing", map[string]any{"projectRoot": "relative-missing"})
	value, err = prefs.Load()
	if err != nil || len(value.Projects) != 125 {
		t.Fatalf("failed removals changed registry: %d, %v", len(value.Projects), err)
	}
}

func projectRequest(t *testing.T, server *mcp.Server, name string, args map[string]any) map[string]any {
	t.Helper()
	request, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args}})
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := server.Serve(context.Background(), strings.NewReader(string(request)+"\n"), &output); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func projectCall(t *testing.T, server *mcp.Server, name string, args map[string]any) map[string]any {
	t.Helper()
	response := projectRequest(t, server, name, args)
	if response["error"] != nil {
		t.Fatalf("%s: %+v", name, response["error"])
	}
	content := response["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		t.Fatal(err)
	}
	return payload
}

func projectCallError(t *testing.T, server *mcp.Server, name string, args map[string]any) {
	t.Helper()
	if response := projectRequest(t, server, name, args); response["error"] == nil {
		t.Fatalf("%s unexpectedly succeeded: %+v", name, response)
	}
}
