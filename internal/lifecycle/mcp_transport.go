package lifecycle

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"time"
)

// OpenMCP opens an authenticated, dedicated session to the desktop owner.
func OpenMCP(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	path, err := leasePath()
	if err != nil {
		return nil, err
	}
	current, err := readLease(path)
	if err != nil {
		return nil, ErrDesktopNotRunning
	}
	conn, err := (&net.Dialer{Timeout: 2 * time.Second}).DialContext(ctx, "tcp", current.Address)
	if err != nil {
		return nil, fmt.Errorf("connect to TestDesk desktop: %w", err)
	}
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	connectionError := func(err error) (net.Conn, error) {
		_ = conn.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if err := authenticate(conn, current.Token); err != nil {
		return connectionError(err)
	}
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	if _, err := fmt.Fprintln(conn, "mcp-v1"); err != nil {
		return connectionError(err)
	}
	response, err := bufio.NewReader(conn).ReadSlice('\n')
	if err != nil || string(response) != "ready\n" {
		return connectionError(errors.New("TestDesk desktop does not have an MCP session handler ready"))
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return connectionError(err)
	}
	if err := ctx.Err(); err != nil {
		return connectionError(err)
	}
	return conn, nil
}
