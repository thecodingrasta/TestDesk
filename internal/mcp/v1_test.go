package mcp_test

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
	"github.com/thecodingrasta/testdesk/internal/mcp"
)

func TestServerExposesV1ToolContract(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mcp.New(mgr).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"testdesk_capabilities", "testdesk_projects", "testdesk_project_activate",
		"testdesk_project_add", "testdesk_tests", "testdesk_run_status",
		"testdesk_artifacts", "testdesk_settings", "testdesk_history",
		"testdesk_test_details", "testdesk_rerun_failed",
	} {
		if !strings.Contains(output.String(), name) {
			t.Fatalf("tools/list did not expose %s: %s", name, output.String())
		}
	}
}

func TestServerExposesAgentRunAndProjectConfigurationTools(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}` + "\n"
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mcp.New(mgr).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"testdesk_project_configure", "testdesk_discover", "testdesk_plan", "testdesk_run", "testdesk_control", "testdesk_wait", "testdesk_diagnostics"} {
		if !strings.Contains(output.String(), name) {
			t.Fatalf("tools/list did not expose %s: %s", name, output.String())
		}
	}
}

func TestServerAddsAndActivatesProject(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	first := copySampleProject(t)
	second := copySampleProject(t)
	mgr, err := manager.NewFromProject(first)
	if err != nil {
		t.Fatal(err)
	}
	rootJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	input := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%s,"activate":true}}}`+"\n"+`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_projects","arguments":{}}}`+"\n", rootJSON)
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mcp.New(mgr).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "activeProjectRoot") || !strings.Contains(output.String(), "projects") {
		t.Fatalf("project tools did not return compact project state: %s", output.String())
	}
}

func TestServerTargetsNamedProjectsWithoutReplacingActiveRunManagers(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	first := copySampleProject(t)
	second := copySampleProject(t)
	setProjectName(t, first, "alpha")
	setProjectName(t, second, "bravo")
	mgr, err := manager.NewFromProject(first)
	if err != nil {
		t.Fatal(err)
	}
	secondRoot, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	input := fmt.Sprintf(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%s,"activate":false}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_project_activate","arguments":{"project":"bravo"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{"project":"alpha"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{"project":"bravo"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"testdesk_run","arguments":{"project":"alpha","query":"ValidOrder"}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"testdesk_run","arguments":{"project":"bravo","query":"ValidOrder"}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"testdesk_wait","arguments":{"project":"alpha","timeoutMs":10000}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"testdesk_wait","arguments":{"project":"bravo","timeoutMs":10000}}}`,
	}, "\n")+"\n", secondRoot)
	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := mcp.New(mgr).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"alpha", "bravo", `\"idle\":true`} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got %s", expected, text)
		}
	}
	if strings.Count(text, `\"idle\":true`) != 2 {
		t.Fatalf("expected independent waits for both project queues, got %s", text)
	}
}

func TestServerGlobalMultiProjectWithoutInitialManager(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	first := copySampleProject(t)
	second := copySampleProject(t)
	setProjectName(t, first, "proj-one")
	setProjectName(t, second, "proj-two")

	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)

	input := fmt.Sprintf(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%s,"activate":false}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%s,"activate":false}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_projects","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{"project":"proj-one"}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{"project":"proj-two"}}}`,
	}, "\n")+"\n", firstJSON, secondJSON)

	var output bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	// Initialize global MCP server with nil initial manager
	if err := mcp.New(nil).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}

	text := output.String()
	for _, expected := range []string{"proj-one", "proj-two", "activeProjectRoot"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got: %s", expected, text)
		}
	}
}

func setProjectName(t *testing.T, root string, name string) {
	t.Helper()
	path := filepath.Join(root, "testdesk.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(data), `"name": "sample-project"`, fmt.Sprintf(`"name": %q`, name), 1)
	if updated == string(data) {
		t.Fatalf("sample project name not found in %s", path)
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestServerHistoryTestDetailsAndRerunFailed(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 1. Discover and run
	if _, err := mgr.Discover(ctx); err != nil {
		t.Fatal(err)
	}
	run, err := mgr.Run(ctx, manager.RunRequest{Filter: domain.Filter{Query: "ValidOrder"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}

	input := fmt.Sprintf(strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_history","arguments":{"limit":10}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_test_details","arguments":{"name":"OrdersControllerTests.Create_ValidOrder_ReturnsCreated"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_rerun_failed","arguments":{"runId":%q}}}`,
	}, "\n")+"\n", run.ID)

	var output bytes.Buffer
	if err := mcp.New(mgr).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"runs", "OrdersControllerTests.Create_ValidOrder_ReturnsCreated", "retestedFromRunId"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got: %s", expected, text)
		}
	}
}

func TestServerDiscoverUpdatesCatalogSync(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Each dependent request is sent only after its predecessor has completed.
	requests := []string{
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_tests","arguments":{"query":"ValidOrder"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_tests","arguments":{"query":"ValidOrder"}}}`,
	}
	server := mcp.New(mgr)
	lines := make([]string, 0, len(requests))
	for _, request := range requests {
		var output bytes.Buffer
		if err := server.Serve(ctx, strings.NewReader(request+"\n"), &output); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, strings.TrimSpace(output.String()))
	}

	// 1st call: 0 matches
	if !strings.Contains(lines[0], `count\":0`) && !strings.Contains(lines[0], `tests\":[]`) {
		t.Fatalf("expected first tests call to have 0 matches: %s", lines[0])
	}
	// 2nd call: discover count 8
	if !strings.Contains(lines[1], `count\":8`) {
		t.Fatalf("expected discover to find 8 tests: %s", lines[1])
	}
	// 3rd call: tests query finds ValidOrder
	if !strings.Contains(lines[2], "Acme.Tests.Orders.CreateOrderTests.ValidOrder_passes") {
		t.Fatalf("expected third tests call to find ValidOrder after discover: %s", lines[2])
	}
}

func TestServerDrainFinalizesBookkeepingOnEOF(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if _, err := mgr.Discover(ctx); err != nil {
		t.Fatal(err)
	}

	// Queue a run and immediately close input stream (EOF)
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_run","arguments":{"query":"ValidOrder"}}}` + "\n"
	var output bytes.Buffer
	if err := mcp.New(mgr).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}

	// Verify recent runs shows completed status rather than stuck queued
	recent, err := mgr.Store().RecentRuns(5)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) == 0 {
		t.Fatal("expected run in history after drain")
	}
	if recent[0].Status != domain.StatusPassed {
		t.Fatalf("expected run status to be %s after drain, got %s", domain.StatusPassed, recent[0].Status)
	}
}
