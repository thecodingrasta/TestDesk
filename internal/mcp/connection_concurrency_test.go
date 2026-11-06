package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type blockingDiscovery struct {
	started  chan struct{}
	release  chan struct{}
	finished chan struct{}
}

func (r blockingDiscovery) Run(ctx context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	if r.finished != nil {
		defer close(r.finished)
	}
	close(r.started)
	select {
	case <-r.release:
		return runner.Result{}, nil
	case <-ctx.Done():
		return runner.Result{}, ctx.Err()
	}
}

func TestConnectionHandlesStatusAndStopWhileWaiting(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	mgr, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "wait", Providers: []domain.Provider{{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := mgr.Control(manager.ControlPause); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.Run(context.Background(), manager.RunRequest{}); err != nil {
		t.Fatal(err)
	}
	server := New(mgr)
	input, send := io.Pipe()
	output, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, writer); _ = writer.Close() }()
	t.Cleanup(func() { cancel(); _ = send.Close(); _ = output.Close(); <-done })
	replies := make(chan response, 4)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var res response
			if decoder.Decode(&res) != nil {
				return
			}
			replies <- res
		}
	}()
	write := func(id int, name string, args string) {
		t.Helper()
		if _, err := fmt.Fprintf(send, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`+"\n", id, name, args); err != nil {
			t.Fatal(err)
		}
	}
	receive := func() response {
		t.Helper()
		select {
		case res := <-replies:
			return res
		case <-time.After(3 * time.Second):
			t.Fatal("connection did not respond while wait was pending")
			return response{}
		}
	}
	write(1, "testdesk_wait", `{"timeoutMs":10000}`)
	write(2, "testdesk_run_status", `{}`)
	if got := receive(); string(got.ID) != "2" || got.Error != nil {
		t.Fatalf("status response: %+v", got)
	}
	write(3, "testdesk_control", `{"action":"stop"}`)
	seen := map[string]bool{}
	for len(seen) < 2 {
		res := receive()
		if res.Error != nil {
			t.Fatalf("request failed: %+v", res)
		}
		seen[string(res.ID)] = true
	}
	if !seen["1"] || !seen["3"] {
		t.Fatalf("missing correlated responses: %v", seen)
	}
}

func TestConnectionHandlesStatusDuringDiscovery(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	r := blockingDiscovery{started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	mgr, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "discovery", Providers: []domain.Provider{{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}}}, r)
	if err != nil {
		t.Fatal(err)
	}
	input, send := io.Pipe()
	output, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- New(mgr).Serve(ctx, input, writer); _ = writer.Close() }()
	t.Cleanup(func() { cancel(); _ = send.Close(); _ = output.Close(); <-done })
	replies := make(chan response, 2)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var res response
			if decoder.Decode(&res) != nil {
				return
			}
			replies <- res
		}
	}()
	if _, err := fmt.Fprintln(send, `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{}}}`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.started:
	case <-time.After(3 * time.Second):
		t.Fatal("discovery did not start")
	}
	if _, err := fmt.Fprintln(send, `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"testdesk_run_status","arguments":{}}}`); err != nil {
		t.Fatal(err)
	}
	select {
	case res := <-replies:
		if string(res.ID) != "11" || res.Error != nil {
			t.Fatalf("status response: %+v", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("status blocked behind discovery")
	}
	close(r.release)
	select {
	case res := <-replies:
		if string(res.ID) != "10" || res.Error != nil {
			t.Fatalf("discovery response: %+v", res)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("discovery response missing")
	}
}

func TestLongRequestKeepsSessionProjectAcrossSwitch(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	newManager := func(name string) *manager.Manager {
		mgr, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: name}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return mgr
	}
	first, second := newManager("first"), newManager("second")
	server := New(first)
	req := request{Method: "tools/call", Params: json.RawMessage(`{"name":"testdesk_wait","arguments":{}}`)}
	ctx := server.bindLongRequestProject(context.Background(), req)
	server.setActiveManager(second)
	got, err := server.managerForLongRequest(ctx, nil)
	if err != nil || got != first {
		t.Fatalf("long request changed project: %v, %v", got, err)
	}
}

func TestDisconnectCancelsPendingDiscovery(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	r := blockingDiscovery{started: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	mgr, err := manager.New(t.TempDir(), domain.ProjectConfig{Version: 1, Name: "disconnect", Providers: []domain.Provider{{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}}}, r)
	if err != nil {
		t.Fatal(err)
	}
	input, send := io.Pipe()
	done := make(chan error, 1)
	go func() { done <- New(mgr).ServeConnection(input, io.Discard) }()
	if _, err := fmt.Fprintln(send, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{}}}`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-r.started:
	case <-time.After(3 * time.Second):
		t.Fatal("discovery did not start")
	}
	_ = send.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("disconnected connection did not exit")
	}
	select {
	case <-r.finished:
	case <-time.After(3 * time.Second):
		t.Fatal("discovery was not cancelled on disconnect")
	}
	// The runner returns before Discover finishes saving state under .testdesk.
	// Wait for Discover itself to return so temporary-directory cleanup cannot race it.
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

func TestConnectionControlsAnotherProjectDuringWait(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	newManager := func(name string) *manager.Manager {
		root := t.TempDir()
		provider := domain.Provider{ID: "fixture", Name: name, Kind: domain.ProviderCustom, Enabled: true, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
		cfg := domain.ProjectConfig{Version: 1, Name: name, Providers: []domain.Provider{provider}}
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
		t.Cleanup(func() {
			_ = mgr.Control(manager.ControlStop)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = mgr.WaitForIdle(ctx)
		})
		return mgr
	}
	first, second := newManager("first"), newManager("second")
	server := New(first)
	server.setActiveManager(second)
	server.setActiveManager(first)
	input, send := io.Pipe()
	output, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, writer); _ = writer.Close() }()
	t.Cleanup(func() { cancel(); _ = send.Close(); _ = output.Close(); <-done })
	replies := make(chan response, 4)
	go func() {
		decoder := json.NewDecoder(output)
		for {
			var res response
			if decoder.Decode(&res) != nil {
				return
			}
			replies <- res
		}
	}()
	write := func(id int, name string, args any) {
		t.Helper()
		raw, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fmt.Fprintf(send, `{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`+"\n", id, name, raw); err != nil {
			t.Fatal(err)
		}
	}
	receive := func() response {
		t.Helper()
		select {
		case res := <-replies:
			if res.Error != nil {
				t.Fatalf("RPC error: %+v", res.Error)
			}
			return res
		case <-time.After(3 * time.Second):
			t.Fatal("request blocked")
			return response{}
		}
	}
	write(1, "testdesk_wait", map[string]any{"timeoutMs": 10000})
	write(2, "testdesk_control", map[string]any{"projectRoot": second.ProjectRoot(), "action": "stop"})
	if res := receive(); string(res.ID) != "2" {
		t.Fatalf("second project control blocked: %+v", res)
	}
	if !first.IsActive() {
		t.Fatal("stopping the second project affected the first")
	}
	write(3, "testdesk_control", map[string]any{"action": "stop"})
	seen := map[string]bool{}
	for len(seen) < 2 {
		seen[string(receive().ID)] = true
	}
	if !seen["1"] || !seen["3"] {
		t.Fatalf("missing responses: %v", seen)
	}
}
