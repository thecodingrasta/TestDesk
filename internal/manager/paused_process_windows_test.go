//go:build windows

package manager_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
)

func TestPausedQueueControlWithRealProcessTreeAndRecovery(t *testing.T) {
	for _, action := range []manager.ControlAction{manager.ControlStop, manager.ControlKill} {
		t.Run(string(action), func(t *testing.T) {
			t.Setenv("TESTDESK_CONFIG_HOME", t.TempDir())
			root := t.TempDir()
			provider := domain.Provider{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Root: ".", Enabled: true, Results: domain.ResultSpec{Format: "none"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: os.Args[0], Args: []string{"-test.run=^TestQueueProcessHelper$"}, Env: map[string]string{"TESTDESK_QUEUE_PROCESS_HELPER": "parent", "TESTDESK_QUEUE_PROCESS_ROOT": root}, TimeoutSeconds: 50}}}
			m, err := manager.New(root, domain.ProjectConfig{Version: 1, Name: "queue", Providers: []domain.Provider{provider}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			var owned []int
			t.Cleanup(func() {
				for _, pid := range owned {
					if queueProcessAlive(pid) {
						_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
					}
				}
			})
			first, err := m.Run(ctx, manager.RunRequest{})
			if err != nil {
				t.Fatal(err)
			}
			firstPIDs := queueWaitPIDs(t, root, "first")
			owned = append(owned, firstPIDs...)
			if err := m.Control(manager.ControlPause); err != nil {
				t.Fatal(err)
			}
			second, err := m.Run(ctx, manager.RunRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if err := m.Control(action); err != nil {
				t.Fatal(err)
			}
			if err := m.WaitForIdle(ctx); err != nil {
				t.Fatal(err)
			}
			queueWaitDead(t, firstPIDs)
			if _, err := os.Stat(filepath.Join(root, "second")); !os.IsNotExist(err) {
				t.Fatalf("queued process unexpectedly started: %v", err)
			}
			for _, id := range []string{first.ID, second.ID} {
				diag, err := m.Diagnostics(id, 1024)
				if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusCancelled {
					t.Fatalf("run %s: %+v, %v", id, diag.Run, err)
				}
			}
			if err := m.Control(manager.ControlResume); err != nil {
				t.Fatal(err)
			}
			third, err := m.Run(ctx, manager.RunRequest{})
			if err != nil {
				t.Fatal(err)
			}
			thirdPIDs := queueWaitPIDs(t, root, "second")
			owned = append(owned, thirdPIDs...)
			if err := m.Control(manager.ControlStop); err != nil {
				t.Fatal(err)
			}
			if err := m.WaitForIdle(ctx); err != nil {
				t.Fatal(err)
			}
			queueWaitDead(t, thirdPIDs)
			diag, err := m.Diagnostics(third.ID, 1024)
			if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusCancelled {
				t.Fatalf("recovery run: %+v, %v", diag.Run, err)
			}
			reopened, err := manager.New(root, domain.ProjectConfig{Version: 1, Name: "queue", Providers: []domain.Provider{provider}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			for _, id := range []string{first.ID, second.ID, third.ID} {
				diag, err := reopened.Diagnostics(id, 1024)
				if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusCancelled {
					t.Fatalf("reopened run %s: %+v, %v", id, diag.Run, err)
				}
			}
		})
	}
}
