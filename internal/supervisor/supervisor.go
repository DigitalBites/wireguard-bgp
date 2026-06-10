package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	DefaultSocketPath = "/run/peplink-wg-bgp/supervisor.sock"
	ActionPing        = "ping"
)

type Request struct {
	Action string `json:"action"`
}

type Response struct {
	OK     bool   `json:"ok"`
	Action string `json:"action"`
	Output string `json:"output,omitempty"`
	Error  string `json:"error,omitempty"`
}

type Server struct {
	SocketPath string
	AllowedUID int
}

func (s Server) Serve(ctx context.Context) error {
	socketPath := s.SocketPath
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o700); err != nil {
		return err
	}
	_ = os.Remove(socketPath)
	listener, err := net.Listen("unix", socketPath)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err := os.Chmod(socketPath, 0o660); err != nil {
		return err
	}

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		_ = listener.Close()
		errCh <- ctx.Err()
	}()

	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				return err
			}
		}
		go s.handle(conn)
	}
}

func (s Server) handle(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	var req Request
	if err := json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Error: "invalid request: " + err.Error()})
		return
	}
	resp := s.dispatch(req)
	_ = json.NewEncoder(conn).Encode(resp)
}

func (s Server) dispatch(req Request) Response {
	switch req.Action {
	case ActionPing:
		return Response{OK: true, Action: req.Action, Output: "pong"}
	default:
		return Response{OK: false, Action: req.Action, Error: fmt.Sprintf("unknown supervisor action %q", req.Action)}
	}
}

type Client struct {
	SocketPath string
	Timeout    time.Duration
}

func (c Client) Call(ctx context.Context, action string) (Response, error) {
	socketPath := c.SocketPath
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(conn).Encode(Request{Action: action}); err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.NewDecoder(bufio.NewReader(conn)).Decode(&resp); err != nil {
		return Response{}, err
	}
	if !resp.OK {
		if resp.Error == "" {
			resp.Error = "supervisor action failed"
		}
		return resp, errors.New(resp.Error)
	}
	return resp, nil
}
