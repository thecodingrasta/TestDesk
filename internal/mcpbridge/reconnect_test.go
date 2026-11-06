package mcpbridge_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/lifecycle"
	"github.com/thecodingrasta/testdesk/internal/mcpbridge"
)

func TestBridgesDisconnectOnOwnerShutdownAndReconnect(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	startOwner := func() *lifecycle.Owner {
		t.Helper()
		owner, err := lifecycle.Start()
		if err != nil {
			t.Fatal(err)
		}
		owner.SetMCPHandler(func(input io.Reader, output io.Writer) { _, _ = io.Copy(output, input) })
		t.Cleanup(func() { _ = owner.Close() })
		return owner
	}
	type client struct {
		input  *io.PipeWriter
		output *io.PipeReader
		done   chan error
	}
	connect := func() *client {
		t.Helper()
		input, send := io.Pipe()
		output, write := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		c := &client{input: send, output: output, done: make(chan error, 1)}
		go func() { c.done <- mcpbridge.Serve(ctx, input, write, "", ""); _ = write.Close() }()
		t.Cleanup(func() { cancel(); _ = send.Close(); _ = output.Close() })
		return c
	}
	exchange := func(c *client, message string) {
		t.Helper()
		reply := make(chan string, 1)
		go func() {
			_, err := fmt.Fprintln(c.input, message)
			if err != nil {
				reply <- "write failed: " + err.Error()
				return
			}
			line, err := bufio.NewReader(c.output).ReadString('\n')
			if err != nil {
				reply <- "read failed: " + err.Error()
				return
			}
			reply <- strings.TrimSpace(line)
		}()
		select {
		case got := <-reply:
			if got != message {
				t.Fatalf("bridge response = %q, want %q", got, message)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("bridge exchange timed out")
		}
	}
	owner := startOwner()
	first, second := connect(), connect()
	exchange(first, "first client")
	exchange(second, "second client")
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []*client{first, second} {
		select {
		case err := <-c.done:
			if err == nil {
				t.Fatal("owner shutdown was not reported to an active client")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("client remained connected after owner shutdown")
		}
	}
	_ = first.input.Close()
	_ = second.input.Close()
	_ = first.output.Close()
	_ = second.output.Close()
	_ = startOwner()
	replacement := connect()
	exchange(replacement, "replacement owner")
	_ = replacement.input.Close()
	select {
	case err := <-replacement.done:
		if err != nil {
			t.Fatalf("normal client disconnect: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("replacement bridge did not close")
	}
}

func TestBridgeRecoversAfterDesktopExecutableCannotStart(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	missingExecutable := filepath.Join(t.TempDir(), "missing-testdesk-desktop")
	var output strings.Builder
	err := mcpbridge.Serve(ctx, strings.NewReader("probe\n"), &output, "", missingExecutable)
	if err == nil || !strings.Contains(err.Error(), "start TestDesk desktop") {
		t.Fatalf("missing desktop executable returned %v; want actionable startup error", err)
	}
	if output.Len() != 0 {
		t.Fatalf("failed startup unexpectedly produced MCP output: %q", output.String())
	}
	owner, err := lifecycle.Start()
	if err != nil {
		t.Fatalf("start owner after failed launch: %v", err)
	}
	defer owner.Close()
	owner.SetMCPHandler(func(input io.Reader, output io.Writer) { _, _ = io.Copy(output, input) })
	if err := mcpbridge.Serve(ctx, strings.NewReader("probe\n"), &output, "", ""); err != nil {
		t.Fatalf("connect to replacement owner: %v", err)
	}
	if output.String() != "probe\n" {
		t.Fatalf("replacement owner response = %q", output.String())
	}
}
