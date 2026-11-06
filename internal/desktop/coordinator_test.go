package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/mcp"
)

func TestDesktopCoordinatorUsesTheSameProjectManager(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	root := t.TempDir()
	provider := config.DotNetProvider()
	provider.Run.All = domain.CommandSpec{Exe: "fixture"}
	provider.Results = domain.ResultSpec{Format: "none"}
	cfg := domain.ProjectConfig{Version: 1, Name: "coordinator", Providers: []domain.Provider{provider}}
	if err := config.Save(root, cfg); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{}, 1)
	mgr, err := manager.New(root, cfg, visibilityRunner{started: started})
	if err != nil {
		t.Fatal(err)
	}
	app := &App{managers: map[string]*manager.Manager{root: mgr}}
	server := mcp.NewWithManagerResolver(app.managerForProject)
	input, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.Serve(ctx, input, io.Discard) }()
	defer writer.Close()
	rootJSON, err := json.Marshal(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_run","arguments":{"project":%s}}}`+"\n", rootJSON); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("MCP did not start the desktop-owned manager")
	}
	if !app.runActive() {
		t.Fatal("desktop did not observe the MCP run")
	}
	if _, err := fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_control","arguments":{"project":%s,"action":"stop"}}}`+"\n", rootJSON); err != nil {
		t.Fatal(err)
	}
	waitCtx, stopWait := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopWait()
	if err := mgr.WaitForIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	if app.runActive() {
		t.Fatal("desktop still reports activity after MCP stopped the run")
	}
	_ = writer.Close()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("MCP session did not finish")
	}
}

func TestDesktopManagerResolutionIsSharedAcrossConcurrentClients(t *testing.T) {
	root := t.TempDir()
	if err := config.Save(root, domain.ProjectConfig{Version: 1, Name: "shared"}); err != nil {
		t.Fatal(err)
	}
	app := &App{}
	const clients = 12
	results := make(chan *manager.Manager, clients)
	errors := make(chan error, clients)
	var wg sync.WaitGroup
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mgr, err := app.managerForProject(root)
			if err != nil {
				errors <- err
				return
			}
			results <- mgr
		}()
	}
	wg.Wait()
	close(results)
	close(errors)
	for err := range errors {
		t.Fatal(err)
	}
	var first *manager.Manager
	for mgr := range results {
		if first == nil {
			first = mgr
		} else if first != mgr {
			t.Fatal("concurrent requests created separate project managers")
		}
	}
	if first == nil {
		t.Fatal("no project manager was returned")
	}
}

func TestMCPDisconnectCancelsOnlyItsOwnProjectRun(t *testing.T) {
	app := &App{managers: make(map[string]*manager.Manager)}
	type client struct {
		mgr    *manager.Manager
		writer *io.PipeWriter
		done   chan error
	}
	clients := make([]client, 0, 2)
	for i := 0; i < 2; i++ {
		root := t.TempDir()
		provider := config.DotNetProvider()
		provider.Run.All = domain.CommandSpec{Exe: "fixture"}
		provider.Results = domain.ResultSpec{Format: "none"}
		cfg := domain.ProjectConfig{Version: 1, Providers: []domain.Provider{provider}}
		if err := config.Save(root, cfg); err != nil {
			t.Fatal(err)
		}
		started := make(chan struct{}, 1)
		mgr, err := manager.New(root, cfg, visibilityRunner{started: started})
		if err != nil {
			t.Fatal(err)
		}
		app.mu.Lock()
		app.managers[root] = mgr
		app.mu.Unlock()
		reader, writer := io.Pipe()
		t.Cleanup(func() { _ = writer.Close() })
		server := mcp.NewWithManagerResolver(app.managerForProject)
		done := make(chan error, 1)
		go func() { done <- server.ServeConnection(reader, io.Discard) }()
		encodedRoot, _ := json.Marshal(root)
		if _, err := fmt.Fprintf(writer, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"testdesk_run","arguments":{"project":%s}}}`+"\n", encodedRoot); err != nil {
			t.Fatal(err)
		}
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatal("client run did not start")
		}
		clients = append(clients, client{mgr: mgr, writer: writer, done: done})
	}
	_ = clients[0].writer.Close()
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := clients[0].mgr.WaitForIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	if !clients[1].mgr.IsActive() || !app.runActive() {
		t.Fatal("disconnect cancelled the other client's work")
	}
	_ = clients[1].writer.Close()
	if err := clients[1].mgr.WaitForIdle(waitCtx); err != nil {
		t.Fatal(err)
	}
	for _, client := range clients {
		select {
		case <-client.done:
		case <-waitCtx.Done():
			t.Fatal("disconnected session did not return")
		}
	}
}
