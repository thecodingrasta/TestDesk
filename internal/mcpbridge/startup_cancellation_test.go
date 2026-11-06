package mcpbridge

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestCancelledStartupDoesNotLaunchDesktop(t *testing.T) {
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// A launch attempt would return an executable-not-found error instead.
	conn, err := connectOrStart(ctx, filepath.Join(t.TempDir(), "not-a-desktop"), "")
	if conn != nil {
		_ = conn.Close()
		t.Fatal("cancelled startup returned a connection")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled startup attempted to launch: %v", err)
	}
}
