package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/thecodingrasta/testdesk/internal/domain"
)

// EventType identifies streamed process output and lifecycle changes.
type EventType string

const (
	EventStarted  EventType = "started"
	EventStdout   EventType = "stdout"
	EventStderr   EventType = "stderr"
	EventFinished EventType = "finished"
)

type Event struct {
	Type      EventType `json:"type"`
	Line      string    `json:"line,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type Result struct {
	ExitCode int
	Stdout   string
	Stderr   string
	Started  time.Time
	Finished time.Time
	TimedOut bool
}

type Runner interface {
	Run(ctx context.Context, command domain.CommandSpec, onEvent func(Event)) (Result, error)
}

type ProcessRunner struct{}

func NewProcessRunner() ProcessRunner { return ProcessRunner{} }

func (ProcessRunner) Run(ctx context.Context, command domain.CommandSpec, onEvent func(Event)) (Result, error) {
	if result, err, handled := runPlatformProcess(ctx, command, onEvent); handled {
		return result, err
	}
	if command.Exe == "" {
		return Result{}, errors.New("empty executable")
	}
	if command.WorkingDir != "" {
		if _, err := os.Stat(command.WorkingDir); err != nil {
			return Result{}, fmt.Errorf("working directory %q: %w", command.WorkingDir, err)
		}
	}

	runCtx := ctx
	var cancel context.CancelFunc
	timedOut := false
	if command.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(command.TimeoutSeconds)*time.Second)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, command.Exe, command.Args...)
	cmd.Dir = command.WorkingDir
	cmd.Env = os.Environ()
	for key, value := range command.Env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}
	configureCommand(cmd)
	cmd.Cancel = func() error {
		return terminateProcessTree(cmd.Process)
	}
	cmd.WaitDelay = 3 * time.Second

	stdout := &lineCapture{eventType: EventStdout, onEvent: onEvent, cancel: cancel}
	stderr := &lineCapture{eventType: EventStderr, onEvent: onEvent, cancel: cancel}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	started := time.Now().UTC()
	if onEvent != nil {
		onEvent(Event{Type: EventStarted, Timestamp: started})
	}
	if err := cmd.Start(); err != nil {
		return Result{}, err
	}

	waitErr := cmd.Wait()
	stdout.finish()
	stderr.finish()
	waitErr = errors.Join(waitErr, stdout.err, stderr.err)
	if runCtx.Err() == context.DeadlineExceeded {
		timedOut = true
	}
	if runCtx.Err() != nil {
		_ = terminateProcessTree(cmd.Process)
	}
	finished := time.Now().UTC()
	if onEvent != nil {
		onEvent(Event{Type: EventFinished, Timestamp: finished})
	}

	exitCode := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if runCtx.Err() != nil {
			exitCode = -1
		} else {
			exitCode = -1
		}
	}
	return Result{ExitCode: exitCode, Stdout: stdout.output.String(), Stderr: stderr.output.String(), Started: started, Finished: finished, TimedOut: timedOut}, waitErr
}

// Each stream is bounded, including an unterminated line. Exceeding the limit
// cancels the command and discards incomplete output so parsers cannot accept it.
const maxCapturedOutput = 32 * 1024 * 1024

type lineCapture struct {
	output    bytes.Buffer
	pending   []byte
	eventType EventType
	onEvent   func(Event)
	cancel    context.CancelFunc
	received  int
	limit     int
	err       error
}

func (c *lineCapture) Write(data []byte) (int, error) {
	limit := c.limit
	if limit <= 0 {
		limit = maxCapturedOutput
	}
	if c.err != nil {
		return len(data), nil
	}
	if len(data) > limit-c.received {
		c.err = fmt.Errorf("%s exceeded the %d-byte output capture limit", c.eventType, limit)
		c.pending = nil
		c.output.Reset()
		if c.cancel != nil {
			c.cancel()
		}
		return len(data), nil
	}
	c.received += len(data)
	c.pending = append(c.pending, data...)
	for {
		end := bytes.IndexByte(c.pending, '\n')
		if end < 0 {
			break
		}
		c.emit(c.pending[:end])
		c.pending = c.pending[end+1:]
	}
	return len(data), nil
}

func (c *lineCapture) finish() {
	if c.err != nil {
		return
	}
	if len(c.pending) > 0 {
		c.emit(c.pending)
		c.pending = nil
	}
}

func (c *lineCapture) emit(line []byte) {
	cleanLine := strings.TrimRight(string(line), "\r\n")
	_, _ = c.output.WriteString(cleanLine + "\n")
	if c.onEvent != nil {
		c.onEvent(Event{Type: c.eventType, Line: cleanLine, Timestamp: time.Now().UTC()})
	}
}

func EnsureDir(path string) error {
	return os.MkdirAll(filepath.Clean(path), 0o755)
}
