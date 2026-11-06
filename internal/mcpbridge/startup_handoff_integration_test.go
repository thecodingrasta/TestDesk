package mcpbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/lifecycle"
)

// The desktop executable normally consumes --agent-start. Intercept before
// the Go test flag parser to exercise the real bridge with a disposable owner.
func init() {
	if os.Getenv("TESTDESK_BRIDGE_HANDOFF_FIXTURE") == "" {
		return
	}
	if os.Getenv("TESTDESK_BRIDGE_HANDOFF_FIXTURE") == "gated" {
		_ = os.WriteFile(os.Getenv("TESTDESK_BRIDGE_HANDOFF_PID")+"."+strconv.Itoa(os.Getpid()), nil, 0600)
	}
	var child *lifecycle.StartupChild
	var err error
	if os.Getenv("TESTDESK_BRIDGE_HANDOFF_FIXTURE") != "legacy" {
		child, err = lifecycle.TakeStartupChild()
		if err != nil || child == nil {
			os.Exit(31)
		}
	}
	owner, err := lifecycle.Start()
	if err != nil {
		os.Exit(32)
	}
	if err := os.WriteFile(os.Getenv("TESTDESK_BRIDGE_HANDOFF_PID"), []byte(strconv.Itoa(os.Getpid())), 0600); err != nil {
		os.Exit(33)
	}
	if os.Getenv("TESTDESK_BRIDGE_HANDOFF_FIXTURE") == "stall" {
		<-child.Done()
		os.Exit(34)
	}
	if os.Getenv("TESTDESK_BRIDGE_HANDOFF_FIXTURE") == "gated" {
		for {
			if _, err := os.Stat(os.Getenv("TESTDESK_BRIDGE_HANDOFF_PID") + ".gate"); err == nil {
				break
			}
			select {
			case <-child.Done():
				os.Exit(36)
			case <-time.After(20 * time.Millisecond):
			}
		}
	}
	if child != nil {
		if err := child.Ready(); err != nil || child.Wait() != nil {
			os.Exit(35)
		}
	}
	owner.SetMCPHandler(func(input io.Reader, output io.Writer) { _, _ = io.Copy(output, input) })
	select {}
}

func handoffFixture(t *testing.T, mode string) (string, string) {
	t.Helper()
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	pidFile := filepath.Join(t.TempDir(), "pid")
	t.Setenv("TESTDESK_BRIDGE_HANDOFF_FIXTURE", mode)
	t.Setenv("TESTDESK_BRIDGE_HANDOFF_PID", pidFile)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return executable, pidFile
}

func handoffFixtureProcess(t *testing.T, pidFile string) *os.Process {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(pidFile)
		if err == nil {
			pid, parseErr := strconv.Atoi(string(data))
			if parseErr != nil || pid <= 0 {
				t.Fatalf("invalid fixture PID: %q", data)
			}
			process, openErr := os.FindProcess(pid)
			if openErr != nil {
				t.Fatal(openErr)
			}
			t.Cleanup(func() { _ = process.Kill(); _ = process.Release() })
			return process
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("startup fixture did not launch")
	return nil
}

func TestStartupHandoffCancelsUnreadyOwner(t *testing.T) {
	executable, pidFile := handoffFixture(t, "stall")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		conn, err := connectOrStart(ctx, executable, "")
		if conn != nil {
			_ = conn.Close()
		}
		done <- err
	}()
	_ = handoffFixtureProcess(t, pidFile)
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("startup cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled startup did not return")
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		owner, err := lifecycle.Start()
		if err == nil {
			_ = owner.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("cancelled owner retained the lifecycle lock")
}

func TestStartupHandoffPreservesCallerDeadline(t *testing.T) {
	executable, pidFile := handoffFixture(t, "stall")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		conn, err := connectOrStart(ctx, executable, "")
		if conn != nil {
			_ = conn.Close()
		}
		done <- err
	}()
	_ = handoffFixtureProcess(t, pidFile)
	if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("startup deadline: %v", err)
	}
}

func TestStartupHandoffCommittedOwnerServesOtherClients(t *testing.T) {
	executable, pidFile := handoffFixture(t, "ready")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := connectOrStart(ctx, executable, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = handoffFixtureProcess(t, pidFile)
	_ = conn.Close()
	other, err := lifecycle.OpenMCP(ctx)
	if err != nil {
		t.Fatalf("committed owner unavailable to another client: %v", err)
	}
	defer other.Close()
	_ = other.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := fmt.Fprintln(other, "shared owner"); err != nil {
		t.Fatal(err)
	}
	buffer := make([]byte, len("shared owner\n"))
	if _, err := io.ReadFull(other, buffer); err != nil || string(buffer) != "shared owner\n" {
		t.Fatalf("shared response %q: %v", buffer, err)
	}
}

func TestStartupHandoffDoesNotKillLegacyOwner(t *testing.T) {
	executable, pidFile := handoffFixture(t, "legacy")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := connectOrStart(ctx, executable, "")
	if err != nil {
		t.Fatal(err)
	}
	_ = handoffFixtureProcess(t, pidFile)
	_ = conn.Close()
	other, err := lifecycle.OpenMCP(ctx)
	if err != nil {
		t.Fatalf("legacy owner was killed: %v", err)
	}
	_ = other.Close()
}

func TestStartupHandoffConcurrentClientRecoversAfterStarterCancels(t *testing.T) {
	executable, pidFile := handoffFixture(t, "gated")
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	secondCtx, cancelSecond := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancelSecond()
	first := make(chan error, 1)
	go func() {
		conn, err := connectOrStart(firstCtx, executable, "")
		if conn != nil {
			_ = conn.Close()
		}
		first <- err
	}()
	_ = handoffFixtureProcess(t, pidFile)
	second := make(chan error, 1)
	go func() {
		conn, err := connectOrStart(secondCtx, executable, "")
		if conn != nil {
			_ = conn.Close()
		}
		second <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		matches, _ := filepath.Glob(pidFile + ".*")
		if len(matches) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	matches, _ := filepath.Glob(pidFile + ".*")
	if len(matches) < 2 {
		t.Fatal("second startup attempt did not begin")
	}
	cancelFirst()
	if err := <-first; err != context.Canceled {
		t.Fatalf("first startup: %v", err)
	}
	if err := os.WriteFile(pidFile+".gate", nil, 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second client failed to recover: %v", err)
		}
		_ = handoffFixtureProcess(t, pidFile)
	case <-secondCtx.Done():
		t.Fatal("second client could not start a replacement owner")
	}
}
