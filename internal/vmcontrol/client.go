package vmcontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

// dialTimeout is short — the controller is local and the only legitimate
// reason a connect would block is the controller being down.
const dialTimeout = 2 * time.Second
const ioTimeout = 6 * time.Second

// Client talks to a foyer-vm-controller on a Unix socket. It is safe to share
// across goroutines: each call opens its own connection.
type Client struct {
	socketPath string
}

func NewClient(socketPath string) *Client {
	if socketPath == "" {
		socketPath = SocketPath
	}
	return &Client{socketPath: socketPath}
}

// Call sends a single request and returns the parsed response.
func (c *Client) Call(action, vm string) (*Response, error) {
	if !IsActionAllowed(action) {
		return nil, fmt.Errorf("action not allowed: %s", action)
	}
	// list/list_info don't take a VM name; everything else must be valid.
	if action != ActionList && action != ActionListInfo && !ValidVMName(vm) {
		return nil, fmt.Errorf("invalid vm name")
	}

	conn, err := net.DialTimeout("unix", c.socketPath, dialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect controller: %w", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(ioTimeout))

	req := Request{Action: action, VM: vm}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := conn.Write(body); err != nil {
		return nil, fmt.Errorf("write: %w", err)
	}
	// Tell the server we're done so it returns immediately.
	if uc, ok := conn.(*net.UnixConn); ok {
		_ = uc.CloseWrite()
	}

	respBytes, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	var resp Response
	if err := json.Unmarshal(respBytes, &resp); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if !resp.OK {
		return &resp, errors.New(resp.Error)
	}
	return &resp, nil
}
