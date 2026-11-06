package lifecycle

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestStartupHandoffHelper(t *testing.T) {
	if os.Getenv("TESTDESK_HANDOFF_HELPER") != "1" {
		return
	}
	c, err := TakeStartupChild()
	if err != nil || c == nil {
		t.Fatalf("take: %v", err)
	}
	defer c.Close()
	if again, err := TakeStartupChild(); err != nil || again != nil {
		t.Fatal("handoff environment not consumed")
	}
	if err := c.Ready(); err != nil {
		t.Fatal(err)
	}
	err = c.Wait()
	if os.Getenv("TESTDESK_EXPECT_EOF") == "1" {
		if err == nil {
			t.Fatal("expected parent EOF")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
}

func startHandoffHelper(t *testing.T, expectEOF bool) (*exec.Cmd, *StartupParent) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestStartupHandoffHelper$")
	cmd.Env = append(os.Environ(), "TESTDESK_HANDOFF_HELPER=1")
	if expectEOF {
		cmd.Env = append(cmd.Env, "TESTDESK_EXPECT_EOF=1")
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	parent, err := NewStartupParent(cmd)
	if err != nil {
		t.Fatal(err)
	}
	err = cmd.Start()
	parent.Started()
	if err != nil {
		parent.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { parent.Close() })
	return cmd, parent
}

func waitHandoffReady(t *testing.T, p *StartupParent) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.WaitHello(ctx); err != nil {
		t.Fatal(err)
	}
	if err := p.WaitReady(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestStartupHandoffCommit(t *testing.T) {
	cmd, p := startHandoffHelper(t, false)
	waitHandoffReady(t, p)
	if err := p.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := p.Reclaim(); err == nil {
		t.Fatal("committed child was reclaimable")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestStartupHandoffParentEOF(t *testing.T) {
	cmd, p := startHandoffHelper(t, true)
	waitHandoffReady(t, p)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestStartupHandoffReclaimAndRecovery(t *testing.T) {
	cmd, p := startHandoffHelper(t, false)
	waitHandoffReady(t, p)
	if err := p.Reclaim(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("reclaimed child exited successfully")
	}
	next, nextParent := startHandoffHelper(t, false)
	waitHandoffReady(t, nextParent)
	if err := nextParent.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := next.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestStartupHandoffFailedCommitCannotReclaim(t *testing.T) {
	cmd, p := startHandoffHelper(t, true)
	waitHandoffReady(t, p)
	if err := p.Close(); err != nil {
		t.Fatal(err)
	}
	if err := p.Commit(); err == nil {
		t.Fatal("commit unexpectedly succeeded on closed channel")
	}
	if err := p.Reclaim(); err == nil {
		t.Fatal("failed commit left child reclaimable")
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}

func TestStartupHandoffManual(t *testing.T) {
	t.Setenv(startupReadEnv, "")
	_ = os.Unsetenv(startupReadEnv)
	_ = os.Unsetenv(startupWriteEnv)
	child, err := TakeStartupChild()
	if child != nil || err != nil {
		t.Fatalf("manual launch: %v, %v", child, err)
	}
}
