package lifecycle

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"
)

func TestConnectRequiresRunningDesktop(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	_, _, err := Connect(context.Background())
	if !errors.Is(err, ErrDesktopNotRunning) {
		t.Fatalf("Connect error = %v, want ErrDesktopNotRunning", err)
	}
}

func TestDesktopOwnerAcrossProcesses(t *testing.T) {
	if os.Getenv("TESTDESK_OWNER_HELPER") == "1" {
		owner, err := Start()
		if err != nil {
			fmt.Println("rejected")
			return
		}
		defer owner.Close()
		fmt.Println("ready")
		_, _ = io.Copy(io.Discard, os.Stdin)
		return
	}
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	type child struct {
		cmd    *exec.Cmd
		stdin  io.WriteCloser
		line   chan string
		waited bool
	}
	children := make([]*child, 0, 2)
	defer func() {
		for _, c := range children {
			_ = c.stdin.Close()
			if !c.waited {
				_ = c.cmd.Process.Kill()
				_ = c.cmd.Wait()
			}
		}
	}()
	for i := 0; i < 2; i++ {
		cmd := exec.Command(os.Args[0], "-test.run=^TestDesktopOwnerAcrossProcesses$")
		cmd.Env = append(os.Environ(), "TESTDESK_OWNER_HELPER=1")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		c := &child{cmd: cmd, stdin: stdin, line: make(chan string, 1)}
		children = append(children, c)
		go func() {
			line, _ := bufio.NewReader(stdout).ReadString('\n')
			c.line <- line
		}()
	}
	var running *child
	for _, c := range children {
		select {
		case line := <-c.line:
			switch line {
			case "ready\n", "ready\r\n":
				if running != nil {
					t.Fatal("two processes acquired the desktop owner")
				}
				running = c
			case "rejected\n", "rejected\r\n":
			default:
				t.Fatalf("unexpected helper output %q", line)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("desktop owner helper timed out")
		}
	}
	if running == nil {
		t.Fatal("neither process acquired the desktop owner")
	}
	if err := running.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = running.cmd.Wait()
	running.waited = true
	owner, err := Start()
	if err != nil {
		t.Fatalf("owner could not restart after process death: %v", err)
	}
	defer owner.Close()
}

func TestConcurrentDesktopStartsHaveOneOwner(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	const attempts = 20
	start := make(chan struct{})
	owners := make(chan *Owner, attempts)
	var wg sync.WaitGroup
	for i := 0; i < attempts; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			owner, err := Start()
			if err == nil {
				owners <- owner
			}
		}()
	}
	close(start)
	wg.Wait()
	close(owners)
	var acquired []*Owner
	for owner := range owners {
		acquired = append(acquired, owner)
	}
	defer func() {
		for _, owner := range acquired {
			_ = owner.Close()
		}
	}()
	if len(acquired) != 1 {
		t.Fatalf("got %d simultaneous owners; want one", len(acquired))
	}
	if err := acquired[0].Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := Start()
	if err != nil {
		t.Fatalf("restart after owner close: %v", err)
	}
	defer restarted.Close()
}

func TestDesktopShutdownCancelsMCPConnection(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	owner, err := Start()
	if err != nil {
		t.Fatalf("Start returned an error: %v", err)
	}
	ctx, cleanup, err := Connect(context.Background())
	if err != nil {
		owner.Close()
		t.Fatalf("Connect returned an error: %v", err)
	}
	defer cleanup()
	if err := owner.Close(); err != nil {
		t.Fatalf("Close returned an error: %v", err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("MCP connection remained active after desktop shutdown")
	}
}

func TestSecondDesktopOwnerIsRejected(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	owner, err := Start()
	if err != nil {
		t.Fatalf("Start returned an error: %v", err)
	}
	defer owner.Close()
	if _, err := Start(); err == nil {
		t.Fatal("second desktop owner unexpectedly started")
	}
}

func TestMCPConnectionRemainsActiveWhileDesktopRuns(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	owner, err := Start()
	if err != nil {
		t.Fatalf("Start returned an error: %v", err)
	}
	defer owner.Close()

	ctx, cleanup, err := Connect(context.Background())
	if err != nil {
		t.Fatalf("Connect returned an error: %v", err)
	}
	defer cleanup()

	// Wait 2.5 seconds to ensure deadlines/timers don't prematurely kill the connection
	select {
	case <-ctx.Done():
		t.Fatal("MCP connection prematurely cancelled while desktop was running")
	case <-time.After(2500 * time.Millisecond):
		// Success: connection is healthy and alive
	}
}
