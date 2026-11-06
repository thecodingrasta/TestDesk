package lifecycle

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"
)

func TestAuthenticatedMCPRoute(t *testing.T) {
	owner := &Owner{token: "fixture-token", done: make(chan struct{}), clients: make(map[net.Conn]struct{})}
	owner.SetMCPHandler(func(input io.Reader, output io.Writer) { _, _ = io.Copy(output, input) })
	server, client := net.Pipe()
	defer client.Close()
	go owner.authorise(server)
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if err := authenticate(client, owner.token); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(client, "mcp-v1"); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(client)
	if line, err := reader.ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("MCP route response %q, %v", line, err)
	}
	if _, err := fmt.Fprintln(client, "request"); err != nil {
		t.Fatal(err)
	}
	if line, err := reader.ReadString('\n'); err != nil || line != "request\n" {
		t.Fatalf("MCP payload %q, %v", line, err)
	}
}

func TestMCPRouteRejectsConnectionsUntilHandlerReady(t *testing.T) {
	owner := &Owner{token: "fixture-token", done: make(chan struct{}), clients: make(map[net.Conn]struct{})}
	server, client := net.Pipe()
	defer client.Close()
	go owner.authorise(server)
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	if err := authenticate(client, owner.token); err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintln(client, "mcp-v1"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(client).ReadString('\n')
	if err != nil || line != "unavailable\n" {
		t.Fatalf("readiness response %q, %v", line, err)
	}
}

func TestOpenMCPLocalSocket(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	owner, err := Start()
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	owner.SetMCPHandler(func(input io.Reader, output io.Writer) { _, _ = io.Copy(output, input) })
	conn, err := OpenMCP(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintln(conn, "socket request"); err != nil {
		t.Fatal(err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil || line != "socket request\n" {
		t.Fatalf("socket response %q, %v", line, err)
	}
}
