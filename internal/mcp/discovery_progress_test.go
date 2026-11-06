package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type cancellableDiscoveryFixture struct {
	calls     atomic.Int32
	started   chan struct{}
	cancelled chan struct{}
}

func (f *cancellableDiscoveryFixture) Run(ctx context.Context, _ domain.CommandSpec, _ func(runner.Event)) (runner.Result, error) {
	switch f.calls.Add(1) {
	case 1:
		return runner.Result{Stdout: "old.case\n"}, nil
	case 2:
		close(f.started)
		<-ctx.Done()
		close(f.cancelled)
		return runner.Result{Stdout: "partial.case\n"}, ctx.Err()
	default:
		return runner.Result{Stdout: "new.case\n"}, nil
	}
}
func TestDiscoveryProgressAndRequestCancellation(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	fixture := &cancellableDiscoveryFixture{started: make(chan struct{}), cancelled: make(chan struct{})}
	p := domain.Provider{ID: "slow", Name: "Slow provider", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: []domain.Provider{p}}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = m.Discover(context.Background()); err != nil {
		t.Fatal(err)
	}
	input, send := io.Pipe()
	output, writer := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- New(m).Serve(ctx, input, writer); _ = writer.Close() }()
	t.Cleanup(func() { cancel(); _ = send.Close(); _ = output.Close(); <-done })
	replies := make(chan map[string]json.RawMessage, 10)
	go func() {
		dec := json.NewDecoder(output)
		for {
			var v map[string]json.RawMessage
			if dec.Decode(&v) != nil {
				return
			}
			replies <- v
		}
	}()
	sendLine := func(s string) {
		t.Helper()
		if _, err := fmt.Fprintln(send, s); err != nil {
			t.Fatal(err)
		}
	}
	recv := func() map[string]json.RawMessage {
		t.Helper()
		select {
		case v := <-replies:
			return v
		case <-time.After(4 * time.Second):
			t.Fatal("response timeout")
			return nil
		}
	}
	sendLine(`{"jsonrpc":"2.0","id":"slow-request","method":"tools/call","params":{"name":"testdesk_discover","arguments":{},"_meta":{"progressToken":"scan-1"}}}`)
	select {
	case <-fixture.started:
	case <-time.After(4 * time.Second):
		t.Fatal("discovery not started")
	}
	progress := recv()
	if string(progress["method"]) != `"notifications/progress"` || !strings.Contains(string(progress["params"]), "Slow provider") || !strings.Contains(string(progress["params"]), "scan-1") {
		t.Fatalf("progress: %s", progress)
	}
	sendLine(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"slow-request"}}`)
	select {
	case <-fixture.cancelled:
	case <-time.After(4 * time.Second):
		t.Fatal("request cancellation did not reach provider")
	}
	sendLine(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{}}}`)
	result := recv()
	if string(result["id"]) != "2" {
		t.Fatalf("unexpected response to cancelled request: %s", result)
	}
	snap, err := m.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snap.Tests) != 1 || snap.Tests[0].FullName != "new.case" {
		t.Fatalf("recovery catalog: %+v %v", snap.Tests, err)
	}
}

func TestCancellationIsScopedToConnection(t *testing.T) {
	fixture := &cancellableDiscoveryFixture{started: make(chan struct{}), cancelled: make(chan struct{})}
	fixture.calls.Store(1)
	p := domain.Provider{ID: "slow", Name: "slow", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: []domain.Provider{p}}, fixture)
	if err != nil {
		t.Fatal(err)
	}
	server := New(m)
	in1, send1 := io.Pipe()
	in2, send2 := io.Pipe()
	out2, writer2 := io.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 2)
	go func() { done <- server.Serve(ctx, in1, io.Discard) }()
	go func() { done <- server.Serve(ctx, in2, writer2); _ = writer2.Close() }()
	t.Cleanup(func() {
		cancel()
		_ = send1.Close()
		_ = send2.Close()
		_ = out2.Close()
		<-done
		<-done
	})
	if _, err := fmt.Fprintln(send1, `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"testdesk_discover","arguments":{}}}`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.started:
	case <-time.After(4 * time.Second):
		t.Fatal("discovery did not start")
	}
	if _, err := fmt.Fprintln(send2, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":7}}`); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(send2, `{"jsonrpc":"2.0","id":8,"method":"ping"}`); err != nil {
		t.Fatal(err)
	}
	var reply map[string]json.RawMessage
	if err := json.NewDecoder(out2).Decode(&reply); err != nil || string(reply["id"]) != "8" {
		t.Fatalf("second connection: %+v %v", reply, err)
	}
	select {
	case <-fixture.cancelled:
		t.Fatal("another connection cancelled the request")
	default:
	}
	if _, err := fmt.Fprintln(send1, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":7}}`); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fixture.cancelled:
	case <-time.After(4 * time.Second):
		t.Fatal("own cancellation did not reach provider")
	}
}

type immediateDiscovery struct{}

func (immediateDiscovery) Run(context.Context, domain.CommandSpec, func(runner.Event)) (runner.Result, error) {
	return runner.Result{Stdout: "case\n"}, nil
}
func TestDiscoveryProgressCountsCompletedProviders(t *testing.T) {
	p1 := domain.Provider{ID: "first", Name: "First", Kind: domain.ProviderCustom, Enabled: true, Discover: domain.DiscoverSpec{Command: domain.CommandSpec{Exe: "fixture"}, Parser: "lines"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: "fixture"}}}
	p2 := p1
	p2.ID = "second"
	p2.Name = "Second"
	m, err := manager.New(t.TempDir(), domain.ProjectConfig{Name: "fixture", Providers: []domain.Provider{p1, p2}}, immediateDiscovery{})
	if err != nil {
		t.Fatal(err)
	}
	values := []int{}
	messages := []string{}
	emit := func(value, total int, message string) {
		if total != 2 {
			t.Errorf("total=%d", total)
		}
		values = append(values, value)
		messages = append(messages, message)
	}
	ctx := context.WithValue(context.Background(), progressEmitterKey{}, emit)
	if _, err := New(m).discoverProject(ctx, nil); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(values) != "[0 1 2]" || !strings.Contains(messages[0], "First") || !strings.Contains(messages[1], "Second") {
		t.Fatalf("progress=%v messages=%v", values, messages)
	}
}
func TestRequestIDKeyRejectsNullAndSeparatesStrings(t *testing.T) {
	if requestIDKey(json.RawMessage(`null`)) != "" || requestIDKey(json.RawMessage(`3`)) == requestIDKey(json.RawMessage(`"3"`)) {
		t.Fatal("request IDs incorrectly resolved")
	}
}
