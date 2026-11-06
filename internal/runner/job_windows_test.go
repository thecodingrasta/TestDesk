//go:build windows

package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
	"golang.org/x/sys/windows"
)

func TestJobProcessHelper(t *testing.T) {
	switch os.Getenv("TESTDESK_JOB_HELPER") {
	case "echo":
		fmt.Fprintln(os.Stdout, os.Getenv("TESTDESK_JOB_VALUE"), os.Args[len(os.Args)-1])
		fmt.Fprintln(os.Stderr, "stderr")
		os.Exit(7)
	case "parent":
		executable, _ := os.Executable()
		child := exec.Command(executable, "-test.run=^TestJobProcessHelper$")
		child.Env = append(os.Environ(), "TESTDESK_JOB_HELPER=child")
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(8)
		}
		fmt.Fprintln(os.Stdout, child.Process.Pid)
		os.Exit(0)
	case "child":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
}

func TestWindowsJobProcessOutputAndExit(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result, err := runWindowsProcess(context.Background(), domain.CommandSpec{Exe: executable, Args: []string{"-test.run=^TestJobProcessHelper$", "argument with spaces"}, Env: map[string]string{"TESTDESK_JOB_HELPER": "echo", "TESTDESK_JOB_VALUE": "hello"}}, nil)
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || result.ExitCode != 7 {
		t.Fatalf("exit: %+v, %v", result, err)
	}
	if !strings.Contains(result.Stdout, "hello argument with spaces") || !strings.Contains(result.Stderr, "stderr") {
		t.Fatalf("output: %+v", result)
	}
}

func TestWindowsJobKillsChildAfterParentExits(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result, err := runWindowsProcess(context.Background(), domain.CommandSpec{Exe: executable, Args: []string{"-test.run=^TestJobProcessHelper$"}, Env: map[string]string{"TESTDESK_JOB_HELPER": "parent"}}, nil)
	if err != nil {
		t.Fatalf("run: %+v, %v", result, err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(result.Stdout))
	if err != nil {
		t.Fatalf("pid from %q: %v", result.Stdout, err)
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return
		}
		t.Fatal(err)
	}
	defer windows.CloseHandle(handle)
	status, err := windows.WaitForSingleObject(handle, 5000)
	if err != nil || status != windows.WAIT_OBJECT_0 {
		t.Fatalf("child %d still alive: status %d, err %v", pid, status, err)
	}
}

func TestWindowsJobCancellationKillsTree(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	result, err := runWindowsProcess(ctx, domain.CommandSpec{Exe: executable, Args: []string{"-test.run=^TestJobProcessHelper$"}, Env: map[string]string{"TESTDESK_JOB_HELPER": "child"}}, nil)
	if !errors.Is(err, context.DeadlineExceeded) || !result.TimedOut {
		t.Fatalf("timeout: %+v, %v", result, err)
	}
}

func TestWindowsJobTerminationFailureReturnsPromptly(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	result, err := runWindowsProcessWithTerminate(ctx, domain.CommandSpec{
		Exe: executable, Args: []string{"-test.run=^TestJobProcessHelper$"},
		Env: map[string]string{"TESTDESK_JOB_HELPER": "child"},
	}, nil, func(windows.Handle, uint32) error { return windows.ERROR_ACCESS_DENIED })
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) || !result.TimedOut {
		t.Fatalf("termination failure: %+v, %v", result, err)
	}
	if time.Since(started) > 8*time.Second {
		t.Fatal("termination failure blocked the runner")
	}
}

func TestWindowsJobCrashHelper(t *testing.T) {
	role := os.Getenv("TESTDESK_JOB_CRASH_ROLE")
	if role == "" {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		os.Exit(20)
	}
	root := os.Getenv("TESTDESK_JOB_CRASH_ROOT")
	switch role {
	case "owner":
		_, _ = NewProcessRunner().Run(context.Background(), domain.CommandSpec{
			Exe: executable, Args: []string{"-test.run=^TestWindowsJobCrashHelper$"},
			Env: map[string]string{"TESTDESK_JOB_CRASH_ROLE": "parent", "TESTDESK_JOB_CRASH_ROOT": root},
		}, nil)
		os.Exit(21)
	case "parent":
		child := exec.Command(executable, "-test.run=^TestWindowsJobCrashHelper$")
		child.Env = append(os.Environ(), "TESTDESK_JOB_CRASH_ROLE=child")
		if err := child.Start(); err != nil {
			os.Exit(22)
		}
		if err := os.WriteFile(filepath.Join(root, "pids"), []byte(fmt.Sprintf("%d %d", os.Getpid(), child.Process.Pid)), 0600); err != nil {
			_ = child.Process.Kill()
			os.Exit(23)
		}
		time.Sleep(time.Minute)
	case "child":
		time.Sleep(time.Minute)
	}
}

func TestWindowsJobKillsTreeWhenOwnerCrashes(t *testing.T) {
	root := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	owner := exec.Command(executable, "-test.run=^TestWindowsJobCrashHelper$")
	owner.Env = append(os.Environ(), "TESTDESK_JOB_CRASH_ROLE=owner", "TESTDESK_JOB_CRASH_ROOT="+root)
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	ownerWaited := false
	t.Cleanup(func() {
		if !ownerWaited {
			_ = owner.Process.Kill()
			_ = owner.Wait()
		}
	})
	var pids []int
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		data, readErr := os.ReadFile(filepath.Join(root, "pids"))
		if readErr == nil {
			var parentPID, childPID int
			if _, scanErr := fmt.Sscanf(string(data), "%d %d", &parentPID, &childPID); scanErr == nil && parentPID > 0 && childPID > 0 {
				pids = []int{parentPID, childPID}
				break
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	if len(pids) != 2 {
		t.Fatal("task-owned process tree did not start")
	}
	var handles []windows.Handle
	defer func() {
		for _, handle := range handles {
			if status, _ := windows.WaitForSingleObject(handle, 0); status == uint32(windows.WAIT_TIMEOUT) {
				_ = windows.TerminateProcess(handle, 1)
			}
			_ = windows.CloseHandle(handle)
		}
	}()
	for _, pid := range pids {
		handle, openErr := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(pid))
		if openErr != nil {
			t.Fatalf("open task-owned process %d: %v", pid, openErr)
		}
		handles = append(handles, handle)
	}
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = owner.Wait()
	ownerWaited = true
	for i, handle := range handles {
		status, waitErr := windows.WaitForSingleObject(handle, 5000)
		if waitErr != nil || status != windows.WAIT_OBJECT_0 {
			t.Fatalf("process %d survived owner crash: status %d, %v", pids[i], status, waitErr)
		}
	}
}
