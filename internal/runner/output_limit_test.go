package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

func TestLineCaptureLimitDiscardsIncompleteOutput(t *testing.T) {
	for _, output := range []string{"first\nsecond\n", "unterminated"} {
		ctx, cancel := context.WithCancel(context.Background())
		capture := &lineCapture{limit: 8, cancel: cancel, eventType: EventStdout}
		_, _ = capture.Write([]byte(output[:4]))
		_, _ = capture.Write([]byte(output[4:]))
		capture.finish()
		if capture.err == nil || ctx.Err() == nil || capture.output.Len() != 0 || len(capture.pending) != 0 {
			t.Fatalf("limit did not cancel and discard partial output: %+v", capture)
		}
		_, _ = capture.Write([]byte("more\n"))
		if capture.output.Len() != 0 {
			t.Fatal("accepted output after limit")
		}
		cancel()
	}
}

func TestOutputLimitHelper(t *testing.T) {
	if os.Getenv("TESTDESK_OUTPUT_LIMIT_HELPER") != "1" {
		return
	}
	chunk := strings.Repeat("x", 1024*1024)
	for i := 0; i < 40; i++ {
		fmt.Print(chunk)
	}
	os.Exit(0)
}

func TestProcessRunnerCancelsExcessiveOutput(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	result, err := NewProcessRunner().Run(ctx, domain.CommandSpec{
		Exe: executable, Args: []string{"-test.run=^TestOutputLimitHelper$"},
		Env: map[string]string{"TESTDESK_OUTPUT_LIMIT_HELPER": "1"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "output capture limit") || result.Stdout != "" {
		t.Fatalf("excess output was not rejected: bytes=%d, err=%v", len(result.Stdout), err)
	}
}
