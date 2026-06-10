package supervisor

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

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
	defer func() {
		_ = listener.Close()
	}()
	if err := os.Chmod(socketPath, 0o660); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		_ = listener.Close()
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
	defer func() {
		_ = conn.Close()
	}()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	var req Request
	if err := json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Error: "invalid request: " + err.Error()})
		return
	}
	resp := s.dispatch(req)
	_ = json.NewEncoder(conn).Encode(resp)
}
