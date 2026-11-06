package mcpbridge_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/lifecycle"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/mcp"
	"github.com/thecodingrasta/testdesk/internal/mcpbridge"
	"github.com/thecodingrasta/testdesk/internal/prefs"
)

func TestBridgeRoutesEachClientToItsSelectedProject(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	owner, err := lifecycle.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	owner.SetMCPHandler(func(input io.Reader, output io.Writer) {
		server := mcp.NewWithManagerResolver(manager.NewFromProject)
		_ = server.ServeConnection(input, output)
	})
	for _, name := range []string{"project-alpha", "project-beta"} {
		root := t.TempDir()
		if err := config.Save(root, domain.ProjectConfig{Version: 1, Name: name}); err != nil {
			t.Fatal(err)
		}
		input, writer := io.Pipe()
		output, reader := io.Pipe()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		done := make(chan error, 1)
		go func() { done <- mcpbridge.Serve(ctx, input, reader, root, "") }()
		_, err := fmt.Fprintln(writer, `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"testdesk_snapshot","arguments":{}}}`)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		line, err := bufio.NewReader(output).ReadString('\n')
		if err != nil || !strings.Contains(line, name) || !strings.Contains(line, `"id":8`) {
			cancel()
			t.Fatalf("project response %q, %v", line, err)
		}
		_ = writer.Close()
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("bridge did not close after client disconnect")
		}
		_ = output.Close()
		_ = reader.Close()
		cancel()
	}
	value, err := prefs.Load()
	if err != nil || len(value.Projects) != 0 || value.LastProjectRoot != "" {
		t.Fatalf("session selection changed saved preferences: %+v, %v", value, err)
	}
}

func TestBridgeAllowsStatusAndControlDuringWait(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	provider := domain.Provider{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	cfg := domain.ProjectConfig{Version: 1, Name: "bridge-fixture", Providers: []domain.Provider{provider}}
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	mgr, err := manager.New(root, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Control(manager.ControlPause); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Run(context.Background(), manager.RunRequest{}); err != nil {
		t.Fatal(err)
	}
	owner, err := lifecycle.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	owner.SetMCPHandler(func(input io.Reader, output io.Writer) {
		_ = mcp.NewWithManagerResolver(func(string) (*manager.Manager, error) { return mgr, nil }).ServeConnection(input, output)
	})
	input, send := io.Pipe()
	output, writer := io.Pipe()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	done := make(chan error, 1)
	go func() { done <- mcpbridge.Serve(ctx, input, writer, root, ""); _ = writer.Close() }()
	t.Cleanup(func() { cancel(); _ = send.Close(); _ = output.Close(); <-done })
	type rpcReply struct {
		ID    int             `json:"id"`
		Error json.RawMessage `json:"error"`
	}
	replies := make(chan rpcReply, 3)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var res rpcReply
			if decoder.Decode(&res) != nil {
				return
			}
			replies <- res
		}
	}()
	write := func(line string) {
		t.Helper()
		if _, err := fmt.Fprintln(send, line); err != nil {
			t.Fatal(err)
		}
	}
	receive := func() rpcReply {
		t.Helper()
		select {
		case res := <-replies:
			if len(res.Error) != 0 {
				t.Fatalf("RPC error: %s", res.Error)
			}
			return res
		case <-time.After(3 * time.Second):
			t.Fatal("bridge response timed out")
			return rpcReply{}
		}
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_wait","arguments":{"timeoutMs":8000}}}`)
	write(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_run_status","arguments":{}}}`)
	if res := receive(); res.ID != 2 {
		t.Fatalf("status was blocked behind wait: %+v", res)
	}
	write(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_control","arguments":{"action":"stop"}}}`)
	seen := map[int]bool{}
	for len(seen) < 2 {
		seen[receive().ID] = true
	}
	if !seen[1] || !seen[3] {
		t.Fatalf("missing responses: %v", seen)
	}
}
