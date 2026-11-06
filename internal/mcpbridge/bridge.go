package mcpbridge

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/thecodingrasta/testdesk/internal/config"
	"github.com/thecodingrasta/testdesk/internal/lifecycle"
)

// Serve connects stdio to the desktop-owned MCP session, starting the desktop
// when necessary. desktopExecutable must name the desktop, not the CLI.
func Serve(ctx context.Context, input io.Reader, output io.Writer, project, desktopExecutable string) error {
	root := strings.TrimSpace(project)
	if root == "" && config.Exists(".") {
		root = "."
	}
	if root != "" {
		var err error
		root, err = filepath.Abs(root)
		if err != nil {
			return err
		}
		if !config.Exists(root) {
			return fmt.Errorf("project %s does not contain %s", root, config.FileName)
		}
	}
	conn, err := connectOrStart(ctx, desktopExecutable, root)
	if err != nil {
		return err
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	reader := bufio.NewReader(conn)
	if root != "" {
		request := map[string]any{"jsonrpc": "2.0", "id": "testdesk-session", "method": "testdesk/session", "params": map[string]string{"projectRoot": root}}
		if err := json.NewEncoder(conn).Encode(request); err != nil {
			return err
		}
		line, err := reader.ReadSlice('\n')
		if err != nil {
			return fmt.Errorf("select desktop project: %w", err)
		}
		var response struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			return fmt.Errorf("read desktop project response: %w", err)
		}
		if response.Error != nil {
			return errors.New(response.Error.Message)
		}
	}
	inputDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(conn, input)
		close(inputDone)
		if tcp, ok := conn.(*net.TCPConn); ok {
			_ = tcp.CloseWrite()
		} else {
			_ = conn.Close()
		}
	}()
	_, err = io.Copy(output, reader)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		select {
		case <-inputDone:
		default:
			return errors.New("TestDesk desktop disconnected while the MCP client was active")
		}
	}
	return err
}

func connectOrStart(ctx context.Context, executable, project string) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, firstErr := lifecycle.OpenMCP(ctx)
	if firstErr == nil {
		if err := ctx.Err(); err != nil {
			_ = conn.Close()
			return nil, err
		}
		return conn, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(executable) == "" {
		return nil, fmt.Errorf("TestDesk desktop is unavailable; set TESTDESK_DESKTOP_EXECUTABLE to its executable: %w", firstErr)
	}
	startupCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	lastErr := firstErr
	retryDelay := 200 * time.Millisecond
	for {
		conn, err, retry := startDesktopAttempt(startupCtx, executable, project)
		if !retry {
			if err != nil && ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return conn, err
		}
		if err != nil {
			lastErr = err
		}
		timer := time.NewTimer(retryDelay)
		select {
		case <-timer.C:
		case <-startupCtx.Done():
			timer.Stop()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("TestDesk desktop did not become tray-ready; verify a graphical desktop session and executable: %w", lastErr)
		}
		if retryDelay < time.Second {
			retryDelay *= 2
			if retryDelay > time.Second {
				retryDelay = time.Second
			}
		}
	}
}

// startDesktopAttempt returns retry=true only after the child has exited.
// Another starter may have held the owner lock and then cancelled, so a
// waiting client must be able to launch a replacement.
func startDesktopAttempt(startupCtx context.Context, executable, project string) (net.Conn, error, bool) {
	args := []string{"--agent-start"}
	if project != "" {
		args = append(args, "--project", project)
	}
	cmd := exec.Command(executable, args...)
	handoff, err := lifecycle.NewStartupParent(cmd)
	if err != nil {
		return nil, fmt.Errorf("prepare desktop startup: %w", err), false
	}
	defer handoff.Close()
	if err := startupCtx.Err(); err != nil {
		return nil, err, false
	}
	err = cmd.Start()
	handoff.Started()
	if err != nil {
		return nil, fmt.Errorf("start TestDesk desktop %s: %w", executable, err), false
	}
	committed := false
	verified := false
	defer func() {
		if verified && !committed {
			_ = handoff.Reclaim()
		}
	}()
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	type startupEvent struct {
		ready bool
		err   error
	}
	events := make(chan startupEvent, 2)
	go func() {
		if err := handoff.WaitHello(startupCtx); err != nil {
			events <- startupEvent{err: err}
			return
		}
		events <- startupEvent{}
		events <- startupEvent{ready: true, err: handoff.WaitReady(startupCtx)}
	}()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	var lastErr error
	for {
		conn, err := lifecycle.OpenMCP(startupCtx)
		if err == nil {
			// The reachable owner may be an older desktop that does not
			// implement the handoff, including the process we launched.
			// Once an owner is available, do not kill that process by PID.
			committed = true
			if err := startupCtx.Err(); err != nil {
				_ = conn.Close()
				return nil, err, false
			}
			return conn, nil, false
		}
		lastErr = err
		select {
		case event := <-events:
			if event.err != nil {
				events = nil
			} else if !event.ready {
				verified = true
			} else {
				events = nil
				if err := startupCtx.Err(); err != nil {
					return nil, err, false
				}
				// A commit is irrevocable even if the write fails: the child may
				// have received it and admitted another client already.
				committed = true
				if err := handoff.Commit(); err != nil {
					return nil, fmt.Errorf("commit desktop startup: %w", err), false
				}
			}
		case exitErr := <-exited:
			if exitErr == nil {
				exitErr = errors.New("desktop exited before MCP readiness")
			}
			return nil, exitErr, true
		case <-startupCtx.Done():
			if errors.Is(startupCtx.Err(), context.Canceled) {
				return nil, startupCtx.Err(), false
			}
			return nil, fmt.Errorf("TestDesk desktop did not become tray-ready; verify a graphical desktop session and executable: %w", lastErr), false
		case <-ticker.C:
		}
	}
}

// DesktopExecutableForCLI finds a separately installed desktop beside the CLI,
// or honors an explicit location supplied by the user.
func DesktopExecutableForCLI() string {
	if override := strings.TrimSpace(os.Getenv("TESTDESK_DESKTOP_EXECUTABLE")); override != "" {
		return override
	}
	self, err := os.Executable()
	if err != nil {
		return ""
	}
	return desktopBeside(self)
}

// desktopBeside returns the desktop executable that sits in the same directory as the CLI at
// path self. Windows packages name it TestDesk.exe (which also matches a renamed testdesk.exe on
// that case-insensitive filesystem) and Linux packages name it testdesk, so both spellings are tried.
func desktopBeside(self string) string {
	names := []string{"TestDesk", "testdesk"}
	if runtime.GOOS == "windows" {
		names = []string{"TestDesk.exe"}
	}
	selfInfo, err := os.Stat(self)
	if err != nil {
		return ""
	}
	for _, name := range names {
		candidate := filepath.Join(filepath.Dir(self), name)
		info, err := os.Stat(candidate)
		if err == nil && !info.IsDir() && !os.SameFile(selfInfo, info) {
			return candidate
		}
	}
	return ""
}
