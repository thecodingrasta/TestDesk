package manager_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"github.com/thecodingrasta/testdesk/internal/runner"
)

type damagedCargoOutputRunner struct{ applied chan struct{} }

func (r damagedCargoOutputRunner) Run(ctx context.Context, cmd domain.CommandSpec, onEvent func(runner.Event)) (runner.Result, error) {
	result, err := runner.NewProcessRunner().Run(ctx, cmd, onEvent)
	if err != nil {
		return result, fmt.Errorf("Cargo fixture command: %w: %s %s", err, result.Stderr, result.Stdout)
	}
	if result.ExitCode != 0 {
		return result, fmt.Errorf("Cargo fixture exit %d: %s", result.ExitCode, result.Stderr)
	}
	for _, arg := range cmd.Args {
		if arg == "--exact" {
			if !strings.Contains(result.Stdout, "test tests::selected ... ok") {
				return result, fmt.Errorf("real Cargo case did not pass")
			}
			result.Stdout += strings.Repeat("x", 4*1024*1024+1) + "\n"
			r.applied <- struct{}{}
			break
		}
	}
	return result, nil
}

func TestRealCargoDamagedOutputDoesNotPersistPass(t *testing.T) {
	if os.Getenv("TESTDESK_REAL_CARGO_TESTS") != "1" {
		t.Skip("requires a configured Rust compiler and linker")
	}
	t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
	t.Setenv("CARGO_NET_OFFLINE", "true")
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "src"), 0o700); err != nil {
		t.Fatal(err)
	}
	for name, contents := range map[string]string{
		"Cargo.toml": "[package]\nname = \"fixture\"\nversion = \"0.1.0\"\nedition = \"2021\"\n",
		"src/lib.rs": `#[cfg(test)] mod tests { #[test] fn selected() { std::fs::write("ran", "yes").unwrap(); } }`,
	} {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	applied := make(chan struct{}, 1)
	cfg := config.Detect(root)
	m, err := manager.New(root, cfg, damagedCargoOutputRunner{applied: applied})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cases, err := m.Discover(ctx)
	if err != nil || len(cases) != 1 {
		t.Fatalf("discovery: %v, %v", cases, err)
	}
	run, err := m.Run(ctx, manager.RunRequest{TestIDs: []string{cases[0].ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-applied:
	default:
		t.Fatal("did not damage real Cargo output")
	}
	if _, err := os.Stat(filepath.Join(root, "ran")); err != nil {
		t.Fatal("real test body did not run:", err)
	}
	diag, err := m.Diagnostics(run.ID, 1024)
	if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusFailed || diag.Run.Passed != 0 {
		t.Fatalf("run: %+v, %v", diag.Run, err)
	}
	reopened, err := manager.New(root, cfg, runner.NewProcessRunner())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := reopened.Snapshot(domain.Filter{Limit: 10})
	if err != nil || len(snapshot.Tests) != 1 || snapshot.Tests[0].LastStatus != domain.StatusUnknown {
		t.Fatalf("persisted: %+v, %v", snapshot.Tests, err)
	}
}
