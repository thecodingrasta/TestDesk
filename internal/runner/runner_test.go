package runner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestRunnerHelperProcess(t *testing.T) {
	switch os.Getenv("TESTDESK_RUNNER_HELPER") {
	case "parent":
		cmd := exec.Command(os.Args[0], "-test.run=^TestRunnerHelperProcess$")
		cmd.Env = append(os.Environ(), "TESTDESK_RUNNER_HELPER=child")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			os.Exit(2)
		}
		if err := os.WriteFile(os.Getenv("TESTDESK_CHILD_PID"), []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
			_ = cmd.Process.Kill()
			os.Exit(3)
		}
		_ = cmd.Process.Release()
		fmt.Print("parent output\n")
		os.Exit(0)
	case "child":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "output":
		fmt.Print(strings.Repeat("x", 128*1024) + "\nlast line")
		os.Exit(0)
	}
}

func TestProcessRunnerBoundsInheritedPipeWait(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	defer func() {
		data, err := os.ReadFile(pidPath)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(string(data))
		if err != nil {
			t.Errorf("invalid task-owned child pid: %v", err)
			return
		}
		if child, err := os.FindProcess(pid); err == nil {
			_ = child.Kill()
			_ = child.Release()
		}
	}()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	result, err := NewProcessRunner().Run(context.Background(), domain.CommandSpec{Exe: exe, Args: []string{"-test.run=^TestRunnerHelperProcess$"}, Env: map[string]string{"TESTDESK_RUNNER_HELPER": "parent", "TESTDESK_CHILD_PID": pidPath}}, nil)
	if runtime.GOOS == "windows" && err != nil {
		t.Fatalf("job-backed runner failed after parent exit: %v", err)
	}
	if runtime.GOOS != "windows" && !errors.Is(err, exec.ErrWaitDelay) {
		t.Fatalf("error = %v; want bounded pipe wait", err)
	}
	if elapsed := time.Since(started); elapsed > 8*time.Second {
		t.Fatalf("inherited pipe blocked runner for %s", elapsed)
	}
	if !strings.Contains(result.Stdout, "parent output") {
		t.Fatalf("lost output: %q", result.Stdout)
	}
}

func TestProcessRunnerPreservesLongAndUnterminatedOutput(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewProcessRunner().Run(context.Background(), domain.CommandSpec{Exe: exe, Args: []string{"-test.run=^TestRunnerHelperProcess$"}, Env: map[string]string{"TESTDESK_RUNNER_HELPER": "output"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != strings.Repeat("x", 128*1024)+"\nlast line\n" {
		t.Fatalf("unexpected output length %d", len(result.Stdout))
	}
}
