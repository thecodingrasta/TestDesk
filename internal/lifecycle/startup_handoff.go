package lifecycle

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

const startupReadEnv = "TESTDESK_STARTUP_READ_HANDLE"
const startupWriteEnv = "TESTDESK_STARTUP_WRITE_HANDLE"

// StartupParent owns the private handoff until Commit transfers ownership.
type StartupParent struct {
	mu          sync.Mutex
	command     *exec.Cmd
	read, write *os.File
	inherited   []*os.File
	committed   bool
}

// NewStartupParent attaches private control channels to an unstarted command.
// Call Started immediately after cmd.Start, including when Start fails, to close
// the parent's copies of the child handles. Close the parent on every path.
func NewStartupParent(cmd *exec.Cmd) (*StartupParent, error) {
	if cmd == nil || cmd.Process != nil {
		return nil, errors.New("startup command must be unstarted")
	}
	childRead, parentWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	parentRead, childWrite, err := os.Pipe()
	if err != nil {
		childRead.Close()
		parentWrite.Close()
		return nil, err
	}
	inherited, readID, writeID, err := attachStartupHandles(cmd, childRead, childWrite)
	if err != nil {
		childRead.Close()
		parentWrite.Close()
		parentRead.Close()
		childWrite.Close()
		return nil, err
	}
	// attachStartupHandles takes ownership of childRead and childWrite on success.
	env := cmd.Environ()
	clean := make([]string, 0, len(env)+2)
	for _, entry := range env {
		if !strings.HasPrefix(strings.ToUpper(entry), startupReadEnv+"=") && !strings.HasPrefix(strings.ToUpper(entry), startupWriteEnv+"=") {
			clean = append(clean, entry)
		}
	}
	cmd.Env = append(clean, startupReadEnv+"="+strconv.FormatUint(readID, 10), startupWriteEnv+"="+strconv.FormatUint(writeID, 10))
	return &StartupParent{command: cmd, read: parentRead, write: parentWrite, inherited: inherited}, nil
}

func (p *StartupParent) Started() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, f := range p.inherited {
		_ = f.Close()
	}
	p.inherited = nil
}

func (p *StartupParent) waitMessage(ctx context.Context, expected string) error {
	done := context.AfterFunc(ctx, func() { _ = p.read.Close() })
	defer done()
	data := make([]byte, len(expected))
	_, err := io.ReadFull(p.read, data)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("startup readiness: %w", err)
	}
	if string(data) != expected {
		return errors.New("invalid startup handoff response")
	}
	return nil
}

// WaitHello confirms the child understands the no-admission-before-commit
// protocol. A legacy desktop must never be reclaimed based only on its PID.
func (p *StartupParent) WaitHello(ctx context.Context) error {
	return p.waitMessage(ctx, "hello\n")
}

// WaitReady waits for initialization after a successful WaitHello.
func (p *StartupParent) WaitReady(ctx context.Context) error {
	return p.waitMessage(ctx, "ready\n")
}

// Commit irrevocably transfers ownership before attempting the write. Even a
// failed write means the caller must never reclaim the process.
func (p *StartupParent) Commit() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.committed {
		return errors.New("startup already committed")
	}
	p.committed = true
	_, err := io.WriteString(p.write, "commit\n")
	return err
}

// Reclaim kills only the directly started child before ownership is committed.
// The caller remains responsible for cmd.Wait. Never call after external
// admission has been enabled, which is prohibited until commit is received.
func (p *StartupParent) Reclaim() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.committed {
		return errors.New("startup already committed")
	}
	if p.command.Process == nil {
		return errors.New("startup process has not started")
	}
	return p.command.Process.Kill()
}

func (p *StartupParent) Close() error {
	p.Started()
	e1 := p.read.Close()
	e2 := p.write.Close()
	return errors.Join(e1, e2)
}

// StartupChild monitors its private control channel immediately upon creation.
type StartupChild struct {
	read, write *os.File
	done        chan struct{}
	mu          sync.Mutex
	err         error
}

// TakeStartupChild consumes the inherited channel environment once. A manual
// launch returns nil, nil. A malformed partial channel is an error.
func TakeStartupChild() (*StartupChild, error) {
	r, rok := os.LookupEnv(startupReadEnv)
	w, wok := os.LookupEnv(startupWriteEnv)
	_ = os.Unsetenv(startupReadEnv)
	_ = os.Unsetenv(startupWriteEnv)
	if !rok && !wok {
		return nil, nil
	}
	if !rok || !wok {
		return nil, errors.New("incomplete startup handoff")
	}
	rid, err := strconv.ParseUint(r, 10, 64)
	if err != nil {
		return nil, errors.New("invalid startup read handle")
	}
	wid, err := strconv.ParseUint(w, 10, 64)
	if err != nil || rid == wid {
		return nil, errors.New("invalid startup write handle")
	}
	read := os.NewFile(uintptr(rid), "startup-control")
	write := os.NewFile(uintptr(wid), "startup-ready")
	if read == nil || write == nil {
		return nil, errors.New("invalid startup handoff handles")
	}
	c := &StartupChild{read: read, write: write, done: make(chan struct{})}
	go func() {
		line, err := bufio.NewReader(read).ReadString('\n')
		if err == nil && line != "commit\n" {
			err = errors.New("invalid startup commit")
		}
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.done)
		_ = read.Close()
	}()
	if _, err := io.WriteString(write, "hello\n"); err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("announce startup handoff: %w", err)
	}
	return c, nil
}

// Ready announces that initialization is complete; it does not admit clients.
func (c *StartupChild) Ready() error { _, err := io.WriteString(c.write, "ready\n"); return err }

// Done closes on commit, parent death or protocol failure; Wait distinguishes them.
func (c *StartupChild) Done() <-chan struct{} { return c.done }
func (c *StartupChild) Wait() error           { <-c.done; c.mu.Lock(); defer c.mu.Unlock(); return c.err }
func (c *StartupChild) Close() error          { return errors.Join(c.read.Close(), c.write.Close()) }
