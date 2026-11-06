package mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/mcp"
)

func createTestProject(t *testing.T, name string, testFileContent string) string {
	t.Helper()
	dir := t.TempDir()
	goMod := fmt.Sprintf("module example.com/%s\n\ngo 1.25\n", strings.ToLower(name))
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(goMod), 0o644); err != nil {
		t.Fatal(err)
	}
	testCode := fmt.Sprintf(`package main

import "testing"

%s
`, testFileContent)
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte(testCode), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Normalise(dir, domain.ProjectConfig{
		Version: 1,
		Name:    name,
		Providers: []domain.Provider{
			{
				ID:       "go-unit",
				Name:     "Go Unit Tests",
				Kind:     domain.ProviderGo,
				Root:     ".",
				Enabled:  true,
				Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "go", Args: []string{"test", "-list", ".*", "./..."}}, Parser: "go-list"},
				Run:      domain.RunSpec{All: domain.CommandSpec{Exe: "go", Args: []string{"test", "-json", "./..."}}, Test: domain.CommandSpec{Exe: "go", Args: []string{"test", "-json", "-run", "^{{selector}}$", "./..."}}},
				Results:  domain.ResultSpec{Format: "go-json-stream"},
			},
		},
	})
	if err := config.Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestConcurrentMultiProjectRuns(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	// Create 3 independent projects with distinct tests
	projAlpha := createTestProject(t, "Project-Alpha", `func TestAlphaOne(t *testing.T) {}
func TestAlphaTwo(t *testing.T) {}`)
	projBeta := createTestProject(t, "Project-Beta", `func TestBetaOne(t *testing.T) {}
func TestBetaTwo(t *testing.T) {}`)
	projGamma := createTestProject(t, "Project-Gamma", `func TestGammaOne(t *testing.T) {}
func TestGammaTwo(t *testing.T) {}`)

	mgrAlpha, err := manager.NewFromProject(projAlpha)
	if err != nil {
		t.Fatal(err)
	}
	server := mcp.New(mgrAlpha)

	// Register all 3 projects via MCP
	registerReqs := []string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%q,"name":"Project-Alpha"}}}`, projAlpha),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%q,"name":"Project-Beta"}}}`, projBeta),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%q,"name":"Project-Gamma"}}}`, projGamma),
	}
	var regOutput bytes.Buffer
	if err := server.Serve(ctx, strings.NewReader(strings.Join(registerReqs, "\n")+"\n"), &regOutput); err != nil {
		t.Fatal(err)
	}

	// Concurrently run tests across all 3 projects simultaneously using goroutines
	type projectResult struct {
		name   string
		output string
		err    error
	}

	projects := []struct {
		name string
		root string
	}{
		{"Project-Alpha", projAlpha},
		{"Project-Beta", projBeta},
		{"Project-Gamma", projGamma},
	}

	resultsChan := make(chan projectResult, len(projects))
	var wg sync.WaitGroup

	for i, proj := range projects {
		wg.Add(1)
		go func(idx int, p struct{ name, root string }) {
			defer wg.Done()
			reqs := []string{
				fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"testdesk_run","arguments":{"project":%q}}}`, idx*10+1, p.name),
				fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"testdesk_wait","arguments":{"project":%q,"timeoutMs":30000}}}`, idx*10+2, p.name),
				fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":"testdesk_history","arguments":{"project":%q,"limit":5}}}`, idx*10+3, p.name),
			}
			var buf bytes.Buffer
			if err := server.Serve(ctx, strings.NewReader(strings.Join(reqs, "\n")+"\n"), &buf); err != nil {
				resultsChan <- projectResult{name: p.name, err: err}
				return
			}
			text := buf.String()
			resultsChan <- projectResult{name: p.name, output: text}
		}(i, proj)
	}

	wg.Wait()
	close(resultsChan)

	// Validate each concurrent run completed without collisions or missing data
	for res := range resultsChan {
		if res.err != nil {
			t.Fatalf("project %s run failed with error: %v", res.name, res.err)
		}
		if !strings.Contains(res.output, `\"status\":\"passed\"`) && !strings.Contains(res.output, `"status":"passed"`) {
			t.Fatalf("project %s did not pass or finish cleanly. Output: %s", res.name, res.output)
		}
		if !strings.Contains(res.output, res.name) {
			t.Fatalf("project %s output did not contain expected project name: %s", res.name, res.output)
		}
	}

	// Verify that each project has its own isolated .testdesk/runs directory and files
	for _, proj := range projects {
		runsDir := filepath.Join(proj.root, ".testdesk", "runs")
		entries, err := os.ReadDir(runsDir)
		if err != nil {
			t.Fatalf("failed reading runs directory for %s: %v", proj.name, err)
		}
		if len(entries) == 0 {
			t.Fatalf("expected at least one run artifact in %s, found 0", runsDir)
		}
	}
}

func TestConcurrentProjectStatusIsolation(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	proj1 := createTestProject(t, "Project-One", `func TestOne(t *testing.T) {}`)
	proj2 := createTestProject(t, "Project-Two", `func TestTwo(t *testing.T) {}`)

	mgr1, err := manager.NewFromProject(proj1)
	if err != nil {
		t.Fatal(err)
	}
	server := mcp.New(mgr1)

	// Register projects
	reqs := []string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%q,"name":"Project-One"}}}`, proj1),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_project_add","arguments":{"projectRoot":%q,"name":"Project-Two"}}}`, proj2),
		// Activate Project-One as session default
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_project_activate","arguments":{"project":"Project-One"}}}`,
		// Query snapshot without explicit project (should return Project-One)
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{"includeTests":false}}}`,
		// Query snapshot with explicit project Project-Two (should return Project-Two)
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{"project":"Project-Two","includeTests":false}}}`,
		// Query snapshot again without explicit project (should still be Project-One, untouched)
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{"includeTests":false}}}`,
	}

	var buf bytes.Buffer
	if err := server.Serve(ctx, strings.NewReader(strings.Join(reqs, "\n")+"\n"), &buf); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != len(reqs) {
		t.Fatalf("expected %d response lines, got %d", len(reqs), len(lines))
	}

	var res4, res5, res6 struct {
		ID     int `json:"id"`
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"result"`
	}

	_ = json.Unmarshal([]byte(lines[3]), &res4)
	_ = json.Unmarshal([]byte(lines[4]), &res5)
	_ = json.Unmarshal([]byte(lines[5]), &res6)

	if !strings.Contains(res4.Result.Content[0].Text, "Project-One") {
		t.Fatalf("response 4 should target default Project-One, got %s", res4.Result.Content[0].Text)
	}
	if !strings.Contains(res5.Result.Content[0].Text, "Project-Two") {
		t.Fatalf("response 5 should target explicit Project-Two, got %s", res5.Result.Content[0].Text)
	}
	if !strings.Contains(res6.Result.Content[0].Text, "Project-One") {
		t.Fatalf("response 6 should remain default Project-One, got %s", res6.Result.Content[0].Text)
	}
}
