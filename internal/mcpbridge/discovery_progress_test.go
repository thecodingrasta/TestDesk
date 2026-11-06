package mcpbridge_test

import (
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
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type bridgeDiscoveryFixture struct{ started, cancelled chan struct{} }

func (f bridgeDiscoveryFixture) Run(ctx context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	close(f.started)
	<-ctx.Done()
	close(f.cancelled)
	return runner.Result{}, ctx.Err()
}
func TestBridgeDiscoveryProgressAndCancellation(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	provider := domain.Provider{ID: "fixture", Name: "bridge provider", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	cfg := domain.ProjectConfig{Version: 1, Name: "bridge", Providers: []domain.Provider{provider}}
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	fixture := bridgeDiscoveryFixture{started: make(chan struct{}), cancelled: make(chan struct{})}
	mgr, err := manager.New(root, cfg, fixture)
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
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	done := make(chan error, 1)
	go func() { done <- mcpbridge.Serve(ctx, input, writer, root, ""); _ = writer.Close() }()
	t.Cleanup(func() { cancel(); _ = send.Close(); _ = output.Close(); <-done })
	responses := make(chan map[string]json.RawMessage, 4)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var v map[string]json.RawMessage
			if decoder.Decode(&v) != nil {
				return
			}
			responses <- v
		}
	}()
	if _, err := fmt.Fprintln(send, `{"jsonrpc":"2.0","id":41,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{},"_meta":{"progressToken":"bridge-token"}}}`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.started:
	case <-time.After(5 * time.Second):
		t.Fatal("discovery did not start")
	}
	select {
	case v := <-responses:
		if string(v["method"]) != `"notifications/progress"` || !strings.Contains(string(v["params"]), "bridge provider") {
			t.Fatalf("progress: %+v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("missing bridge progress")
	}
	if _, err := fmt.Fprintln(send, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":41}}`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("bridge cancellation did not reach discovery")
	}
	if _, err := fmt.Fprintln(send, `{"jsonrpc":"2.0","id":42,"method":"ping"}`); err != nil {
		t.Fatal(err)
	}
	select {
	case v := <-responses:
		if string(v["id"]) != "42" {
			t.Fatalf("unexpected reply: %+v", v)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("bridge did not respond after cancellation")
	}
	// The shared owner does not drain cancelled work, and the fixture returns before Discover has
	// finished saving the catalog under .testdesk. Wait for Discover itself to unwind so that
	// temporary-directory cleanup cannot race that write.
	deadline := time.Now().Add(5 * time.Second)
	for mgr.DiscoveryStatus().Active {
		if time.Now().After(deadline) {
			t.Fatal("discovery did not finish unwinding after cancellation")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if outcome := mgr.DiscoveryStatus().Outcome; outcome != "cancelled" {
		t.Fatalf("discovery outcome = %q, want cancelled", outcome)
	}
}
