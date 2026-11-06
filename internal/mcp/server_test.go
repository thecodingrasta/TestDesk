package mcp_test

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/mcp"
)

func TestServerInitializeListAndSnapshot(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{"includeTests":false}}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := mcp.New(mgr).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"protocolVersion", "testdesk_snapshot", "sample-project"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got %s", expected, text)
		}
	}
}

func TestServerResourcesAndPrompts(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	input := strings.Join([]string{
		`{"jsonrpc":"2.0","id":1,"method":"resources/list","params":{}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"testdesk://guide"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"prompts/list","params":{}}`,
		`{"jsonrpc":"2.0","id":4,"method":"prompts/get","params":{"name":"testdesk-verify-changes","arguments":{"module":"auth"}}}`,
	}, "\n") + "\n"
	var output bytes.Buffer
	if err := mcp.New(mgr).Serve(ctx, strings.NewReader(input), &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, expected := range []string{"testdesk://guide", "testdesk-verify-changes", "testdesk_snapshot", "testdesk_run"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected output to contain %q, got %s", expected, text)
		}
	}
}

func TestServerStopsWhenDesktopLifecycleIsCancelled(t *testing.T) {
	project := copySampleProject(t)
	mgr, err := manager.NewFromProject(project)
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- mcp.New(mgr).Serve(ctx, reader, io.Discard)
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned an error after lifecycle cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("MCP server remained blocked on stdin after lifecycle cancellation")
	}
}

func copySampleProject(t *testing.T) string {
	t.Helper()
	repoRoot := findRepoRoot(t)
	source := filepath.Join(repoRoot, "examples", "sample-project")
	target := filepath.Join(t.TempDir(), "sample-project")
	if err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if rel == ".testdesk" || strings.HasPrefix(filepath.ToSlash(rel), ".testdesk/") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dest := filepath.Join(target, rel)
		if entry.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		return os.WriteFile(dest, data, 0o644)
	}); err != nil {
		t.Fatal(err)
	}
	return target
}

func findRepoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(cwd, "go.mod")); err == nil {
			return cwd
		}
		parent := filepath.Dir(cwd)
		if parent == cwd {
			t.Fatal("repo root not found")
		}
		cwd = parent
	}
}
