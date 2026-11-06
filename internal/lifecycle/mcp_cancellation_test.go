package lifecycle

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"
)

func TestOpenMCPCancellationInterruptsHandshake(t *testing.T) {
	for _, phase := range []string{"authentication", "readiness"} {
		t.Run(phase, func(t *testing.T) {
			t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			path, err := leasePath()
			if err != nil {
				t.Fatal(err)
			}
			if err := writeLease(path, lease{Address: listener.Addr().String(), Token: "fixture-token"}); err != nil {
				t.Fatal(err)
			}
			ready := make(chan struct{})
			go func() {
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				reader := bufio.NewReader(conn)
				_, _ = reader.ReadString('\n')
				if phase == "readiness" {
					_, _ = fmt.Fprintln(conn, "ok")
					_, _ = reader.ReadString('\n')
				}
				close(ready)
				_, _ = reader.ReadByte()
			}()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				conn, err := OpenMCP(ctx)
				if conn != nil {
					_ = conn.Close()
				}
				done <- err
			}()
			select {
			case <-ready:
			case <-time.After(3 * time.Second):
				t.Fatal("fixture handshake did not start")
			}
			cancel()
			select {
			case err := <-done:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("wrong cancellation result: %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("handshake did not stop after cancellation")
			}
		})
	}
}
