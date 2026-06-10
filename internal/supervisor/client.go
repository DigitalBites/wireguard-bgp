package supervisor

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"net"
	"time"
)

func (c Client) Call(ctx context.Context, action string) (Response, error) {
	return c.CallWithParams(ctx, action, nil)
}

func (c Client) CallWithParams(ctx context.Context, action string, params map[string]string) (Response, error) {
	socketPath := c.SocketPath
	if socketPath == "" {
		socketPath = DefaultSocketPath
	}
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	dialer := net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "unix", socketPath)
	if err != nil {
		return Response{}, err
	}
	defer func() {
		_ = conn.Close()
	}()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if err := json.NewEncoder(conn).Encode(Request{Action: action, Params: params}); err != nil {
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
