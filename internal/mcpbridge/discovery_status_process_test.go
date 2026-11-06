package mcpbridge_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/lifecycle"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/mcp"
	"github.com/thecodingrasta/testdesk/internal/mcpbridge"
)

func TestDiscoveryStatusProcessHelper(t *testing.T) {
	if os.Getenv("TESTDESK_STATUS_PROCESS_HELPER") != "1" {
		return
	}
	fmt.Println("fixture discovery output")
	time.Sleep(2 * time.Minute)
}
func TestBridgeStatusAndCancellationWithRealProcess(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	provider := domain.Provider{ID: "real", Name: "real process", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: exe, Args: []string{"-test.run=^TestDiscoveryStatusProcessHelper$"}, Env: map[string]string{"TESTDESK_STATUS_PROCESS_HELPER": "1"}, TimeoutSeconds: 60}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: exe}}}
	cfg := domain.ProjectConfig{Version: 1, Name: "fixture", Providers: []domain.Provider{provider}}
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	mgr, err := manager.New(root, cfg, nil)
	if err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	done := make(chan error, 1)
	go func() { done <- mcpbridge.Serve(ctx, input, writer, root, ""); _ = writer.Close() }()
	t.Cleanup(func() { cancel(); _ = send.Close(); _ = output.Close(); <-done })
	replies := make(chan map[string]json.RawMessage, 20)
	go func() {
		d := json.NewDecoder(output)
		for {
			var v map[string]json.RawMessage
			if d.Decode(&v) != nil {
				return
			}
			replies <- v
		}
	}()
	write := func(line string) {
		t.Helper()
		if _, err := fmt.Fprintln(send, line); err != nil {
			t.Fatal(err)
		}
	}
	receive := func() map[string]json.RawMessage {
		t.Helper()
		select {
		case v := <-replies:
			return v
		case <-time.After(5 * time.Second):
			t.Fatal("response timeout")
			return nil
		}
	}
	write(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{}}}`)
	deadline := time.Now().Add(7 * time.Second)
	found := false
	for time.Now().Before(deadline) {
		write(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_discovery_status","arguments":{}}}`)
		response := receive()
		if string(response["id"]) != "2" {
			t.Fatalf("unexpected reply: %+v", response)
		}
		if strings.Contains(string(response["result"]), "lastOutputAt") {
			found = true
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !found {
		t.Fatal("real provider output was not observed")
	}
	write(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`)
	deadline = time.Now().Add(7 * time.Second)
	cancelled := false
	for time.Now().Before(deadline) {
		write(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"testdesk_discovery_status","arguments":{}}}`)
		response := receive()
		if string(response["id"]) != "3" {
			t.Fatalf("unexpected reply: %+v", response)
		}
		if strings.Contains(string(response["result"]), `\"cancelled\"`) {
			cancelled = true
			break
		}
		time.Sleep(40 * time.Millisecond)
	}
	if !cancelled {
		t.Fatal("real provider did not finish after cancellation")
	}
}
