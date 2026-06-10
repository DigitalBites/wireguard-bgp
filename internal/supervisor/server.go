package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

func (s Server) Serve(ctx context.Context) error {
	s = s.withRuntimeDefaults()
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
	if err := s.authorize(conn); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Error: err.Error()})
		return
	}
	_ = conn.SetDeadline(time.Now().Add(60 * time.Second))
	var req Request
	if err := json.NewDecoder(io.LimitReader(conn, 4096)).Decode(&req); err != nil {
		_ = json.NewEncoder(conn).Encode(Response{OK: false, Error: "invalid request: " + err.Error()})
		return
	}
	if s.ActionLock != nil {
		s.ActionLock.Lock()
		defer s.ActionLock.Unlock()
	}
	resp := s.dispatch(req)
	_ = json.NewEncoder(conn).Encode(resp)
}

func (s Server) withRuntimeDefaults() Server {
	if s.WGManager == nil {
		s.WGManager = &WGProcessManager{Runner: s.Runner}
	}
	if s.RouteManager == nil {
		s.RouteManager = &RouteApplyManager{
			AppConfigPath: s.appConfigPath(),
			WGConfigPath:  DefaultWGConfigPath,
			Runner:        s.Runner,
		}
	}
	if s.BIRDManager == nil {
		s.BIRDManager = &BIRDProcessManager{
			ConfigPath: s.birdConfigPath(),
			SocketPath: s.birdSocketPath(),
			Runner:     s.Runner,
		}
	}
	if s.ActionLock == nil {
		s.ActionLock = &sync.Mutex{}
	}
	return s
}

func (s Server) authorize(conn net.Conn) error {
	if s.AllowedUID <= 0 {
		return nil
	}
	uid, err := peerUID(conn)
	if err != nil {
		return err
	}
	if uid != s.AllowedUID {
		return fmt.Errorf("unauthorized supervisor peer uid %d", uid)
	}
	return nil
}

func peerUID(conn net.Conn) (int, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return 0, fmt.Errorf("supervisor connection is not a Unix socket")
	}
	raw, err := unixConn.SyscallConn()
	if err != nil {
		return 0, err
	}
	var uid int
	var controlErr error
	if err := raw.Control(func(fd uintptr) {
		cred, err := syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
		if err != nil {
			controlErr = err
			return
		}
		uid = int(cred.Uid)
	}); err != nil {
		return 0, err
	}
	if controlErr != nil {
		return 0, controlErr
	}
	return uid, nil
}
