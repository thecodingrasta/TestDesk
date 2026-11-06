//go:build windows

package manager_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"github.com/thecodingrasta/testdesk/internal/manager"
	"golang.org/x/sys/windows"
)

func TestQueueProcessHelper(t *testing.T) {
	role := os.Getenv("TESTDESK_QUEUE_PROCESS_HELPER")
	if role == "" {
		return
	}
	if role == "child" {
		time.Sleep(time.Minute)
		return
	}
	root := os.Getenv("TESTDESK_QUEUE_PROCESS_ROOT")
	name := "first"
	if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
		name = "second"
		if err := os.Mkdir(filepath.Join(root, name), 0700); err != nil {
			os.Exit(2)
		}
	}
	child := exec.Command(os.Args[0], "-test.run=^TestQueueProcessHelper$")
	child.Env = append(os.Environ(), "TESTDESK_QUEUE_PROCESS_HELPER=child")
	if err := child.Start(); err != nil {
		os.Exit(3)
	}
	if err := os.WriteFile(filepath.Join(root, name, "pids"), []byte(strconv.Itoa(os.Getpid())+" "+strconv.Itoa(child.Process.Pid)), 0600); err != nil {
		_ = child.Process.Kill()
		os.Exit(4)
	}
	time.Sleep(time.Minute)
}

func TestQueuedCallerCancellationWithRealProcessTrees(t *testing.T) {
	root := t.TempDir()
	p := domain.Provider{ID: "fixture", Name: "fixture", Kind: domain.ProviderCustom, Root: ".", Enabled: true, Results: domain.ResultSpec{Format: "none"}, Run: domain.RunSpec{All: domain.CommandSpec{Exe: os.Args[0], Args: []string{"-test.run=^TestQueueProcessHelper$"}, Env: map[string]string{"TESTDESK_QUEUE_PROCESS_HELPER": "parent", "TESTDESK_QUEUE_PROCESS_ROOT": root}, TimeoutSeconds: 50}}}
	m, err := manager.New(root, domain.ProjectConfig{Version: 1, Name: "queue", Providers: []domain.Provider{p}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	firstCtx, cancelFirst := context.WithCancel(ctx)
	defer cancelFirst()
	secondCtx, cancelSecond := context.WithCancel(ctx)
	defer cancelSecond()
	var owned []int
	t.Cleanup(func() {
		for _, pid := range owned {
			if queueProcessAlive(pid) {
				_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid)).Run()
			}
		}
	})
	first, err := m.Run(firstCtx, manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	firstPIDs := queueWaitPIDs(t, root, "first")
	owned = append(owned, firstPIDs...)
	second, err := m.Run(secondCtx, manager.RunRequest{})
	if err != nil {
		t.Fatal(err)
	}
	cancelFirst()
	secondPIDs := queueWaitPIDs(t, root, "second")
	owned = append(owned, secondPIDs...)
	queueWaitDead(t, firstPIDs)
	for _, pid := range secondPIDs {
		if !queueProcessAlive(pid) {
			t.Fatalf("second caller's process %d did not survive first cancellation", pid)
		}
	}
	cancelSecond()
	if err := m.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	queueWaitDead(t, secondPIDs)
	for _, id := range []string{first.ID, second.ID} {
		diag, err := m.Diagnostics(id, 1024)
		if err != nil || diag.Run == nil || diag.Run.Status != domain.StatusCancelled {
			t.Fatalf("run %s: %+v %v", id, diag.Run, err)
		}
	}
}

func queueWaitPIDs(t *testing.T, root, name string) []int {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(filepath.Join(root, name, "pids"))
		if err == nil {
			fields := strings.Fields(string(data))
			if len(fields) == 2 {
				first, err1 := strconv.Atoi(fields[0])
				second, err2 := strconv.Atoi(fields[1])
				if err1 == nil && err2 == nil && first > 0 && second > 0 {
					return []int{first, second}
				}
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("%s process tree did not start", name)
	return nil
}

func queueProcessAlive(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	return windows.GetExitCodeProcess(handle, &code) == nil && code == 259
}

func queueWaitDead(t *testing.T, pids []int) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		alive := false
		for _, pid := range pids {
			alive = alive || queueProcessAlive(pid)
		}
		if !alive {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("task-owned processes remain: %v", pids)
}
