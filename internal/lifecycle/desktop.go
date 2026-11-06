package lifecycle

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/thecodingrasta/testdesk/internal/prefs"
)

const leaseFileName = "desktop-lifecycle.json"

var ErrDesktopNotRunning = errors.New("TestDesk is not open; launch TestDesk or keep it running in the system tray before using its MCP")

type lease struct {
	Address string `json:"address"`
	Token   string `json:"token"`
	PID     int    `json:"pid"`
}

type Owner struct {
	listener   net.Listener
	token      string
	path       string
	done       chan struct{}
	close      sync.Once
	mu         sync.Mutex
	clients    map[net.Conn]struct{}
	mcpHandler func(io.Reader, io.Writer)
	ownerLock  *os.File
}

// SetMCPHandler installs the desktop's per-connection MCP session handler.
func (o *Owner) SetMCPHandler(handler func(io.Reader, io.Writer)) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.mcpHandler = handler
}

func Start() (*Owner, error) {
	path, err := leasePath()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	lock, err := claimOwnerLock(path + ".lock")
	if err != nil {
		return nil, err
	}
	claimed := false
	defer func() {
		if !claimed {
			_ = lock.Close()
		}
	}()
	if existing, readErr := readLease(path); readErr == nil && existing.available() {
		return nil, errors.New("TestDesk is already running")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("start TestDesk lifecycle listener: %w", err)
	}
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		_ = listener.Close()
		return nil, fmt.Errorf("create TestDesk lifecycle token: %w", err)
	}
	owner := &Owner{
		listener:  listener,
		token:     hex.EncodeToString(tokenBytes),
		path:      path,
		done:      make(chan struct{}),
		clients:   map[net.Conn]struct{}{},
		ownerLock: lock,
	}
	if err := writeLease(path, lease{Address: listener.Addr().String(), Token: owner.token, PID: os.Getpid()}); err != nil {
		_ = listener.Close()
		return nil, err
	}
	go owner.accept()
	claimed = true
	return owner, nil
}

func (o *Owner) Close() error {
	var closeErr error
	o.close.Do(func() {
		close(o.done)
		closeErr = o.listener.Close()
		o.mu.Lock()
		for client := range o.clients {
			_ = client.Close()
		}
		o.clients = map[net.Conn]struct{}{}
		o.mu.Unlock()
		current, err := readLease(o.path)
		if err == nil && current.Token == o.token {
			_ = os.Remove(o.path)
		}
		if o.ownerLock != nil {
			if err := o.ownerLock.Close(); closeErr == nil {
				closeErr = err
			}
		}
	})
	if errors.Is(closeErr, net.ErrClosed) {
		return nil
	}
	return closeErr
}

func Connect(parent context.Context) (context.Context, func(), error) {
	path, err := leasePath()
	if err != nil {
		return nil, nil, err
	}
	current, err := readLease(path)
	if err != nil {
		return nil, nil, ErrDesktopNotRunning
	}
	conn, err := net.DialTimeout("tcp", current.Address, 2*time.Second)
	if err != nil {
		return nil, nil, ErrDesktopNotRunning
	}
	if err := authenticate(conn, current.Token); err != nil {
		_ = conn.Close()
		return nil, nil, ErrDesktopNotRunning
	}
	ctx, cancel := context.WithCancel(parent)
	go func() {
		defer cancel()
		buf := make([]byte, 1)
		for {
			_, readErr := conn.Read(buf)
			if readErr != nil {
				return
			}
		}
	}()
	cleanup := func() {
		cancel()
		_ = conn.Close()
	}
	return ctx, cleanup, nil
}

func (o *Owner) accept() {
	for {
		conn, err := o.listener.Accept()
		if err != nil {
			return
		}
		go o.authorise(conn)
	}
}

func (o *Owner) authorise(conn net.Conn) {
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	reader := bufio.NewReader(conn)
	token, err := reader.ReadSlice('\n')
	if err != nil || strings.TrimSpace(string(token)) != o.token {
		_ = conn.Close()
		return
	}
	if _, err := conn.Write([]byte("ok\n")); err != nil {
		_ = conn.Close()
		return
	}
	_ = conn.SetDeadline(time.Time{})
	o.mu.Lock()
	select {
	case <-o.done:
		o.mu.Unlock()
		_ = conn.Close()
		return
	default:
		o.clients[conn] = struct{}{}
	}
	o.mu.Unlock()

	mode, readErr := reader.ReadSlice('\n')
	if readErr == nil && string(mode) == "mcp-v1\n" {
		o.mu.Lock()
		handler := o.mcpHandler
		o.mu.Unlock()
		if handler != nil {
			if _, err := io.WriteString(conn, "ready\n"); err == nil {
				handler(reader, conn)
			}
		} else {
			_, _ = io.WriteString(conn, "unavailable\n")
		}
	}
	o.mu.Lock()
	delete(o.clients, conn)
	o.mu.Unlock()
	_ = conn.Close()
}

func authenticate(conn net.Conn, token string) error {
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintln(conn, token); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	response, err := reader.ReadString('\n')
	if err != nil || strings.TrimSpace(response) != "ok" {
		return ErrDesktopNotRunning
	}
	return conn.SetDeadline(time.Time{})
}

func (current lease) available() bool {
	conn, err := net.DialTimeout("tcp", current.Address, 750*time.Millisecond)
	if err != nil {
		return false
	}
	defer conn.Close()
	return authenticate(conn, current.Token) == nil
}

func leasePath() (string, error) {
	preferencePath, err := prefs.Path()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(preferencePath), leaseFileName), nil
}

func readLease(path string) (lease, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return lease{}, err
	}
	var current lease
	if err := json.Unmarshal(data, &current); err != nil {
		return lease{}, err
	}
	if strings.TrimSpace(current.Address) == "" || strings.TrimSpace(current.Token) == "" {
		return lease{}, errors.New("invalid TestDesk lifecycle lease")
	}
	return current, nil
}

func writeLease(path string, current lease) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(current)
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	_ = os.Remove(path)
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return nil
}
