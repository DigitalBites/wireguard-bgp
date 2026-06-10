package supervisor

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

type BIRDManager interface {
	Start(ctx context.Context) (string, error)
}

type BIRDProcessManager struct {
	ConfigPath string
	SocketPath string
	Runner     CommandRunner
	Starter    ProcessStarter

	mu      sync.Mutex
	process Process
}

func (s Server) startBIRD() Response {
	ctx, cancel := s.commandContext()
	defer cancel()
	out, err := s.birdManager().Start(ctx)
	if err != nil {
		return Response{OK: false, Action: ActionBIRDStart, Output: out, Error: err.Error()}
	}
	return Response{OK: true, Action: ActionBIRDStart, Output: out}
}

func (s Server) stopBIRD() Response {
	return s.runCommand(ActionBIRDStop, "birdc", "-s", s.birdSocketPath(), "down")
}

func (s Server) reloadBIRD() Response {
	return s.runCommand(ActionBIRDReload, "birdc", "-s", s.birdSocketPath(), "configure")
}

func (s Server) statusBIRD() Response {
	return s.runCommand(ActionBIRDStatus, "birdc", "-s", s.birdSocketPath(), "show", "protocols")
}

func (s Server) detailsBIRD() Response {
	return s.runCommand(ActionBIRDDetails, "birdc", "-s", s.birdSocketPath(), "show", "protocols", "all", "peplink")
}

func (s Server) birdManager() BIRDManager {
	if s.BIRDManager != nil {
		return s.BIRDManager
	}
	return &BIRDProcessManager{
		ConfigPath: s.birdConfigPath(),
		SocketPath: s.birdSocketPath(),
		Runner:     s.Runner,
	}
}

func (m *BIRDProcessManager) Start(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out strings.Builder
	if status, err := m.status(ctx); err == nil {
		out.WriteString(status)
		if !strings.HasSuffix(status, "\n") {
			out.WriteByte('\n')
		}
		out.WriteString("bird already running\n")
		return out.String(), nil
	}
	starter := m.starter()
	proc, err := starter.StartProcess("bird", "-c", m.configPath(), "-s", m.socketPath())
	if err != nil {
		return out.String(), fmt.Errorf("start bird: %w", err)
	}
	m.process = proc
	for i := 0; i < 20; i++ {
		status, err := m.status(ctx)
		if err == nil {
			out.WriteString(status)
			if !strings.HasSuffix(status, "\n") {
				out.WriteByte('\n')
			}
			return out.String(), nil
		}
		select {
		case <-ctx.Done():
			return out.String(), ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
	return out.String(), fmt.Errorf("bird did not become ready")
}

func (m *BIRDProcessManager) status(ctx context.Context) (string, error) {
	return m.runner().Run(ctx, "birdc", "-s", m.socketPath(), "show", "status")
}

func (s Server) birdConfigPath() string {
	if s.BIRDConfigPath == "" {
		return DefaultBIRDConfigPath
	}
	return s.BIRDConfigPath
}

func (m *BIRDProcessManager) configPath() string {
	if m.ConfigPath == "" {
		return DefaultBIRDConfigPath
	}
	return m.ConfigPath
}

func (m *BIRDProcessManager) socketPath() string {
	if m.SocketPath == "" {
		return DefaultBIRDSocketPath
	}
	return m.SocketPath
}

func (m *BIRDProcessManager) runner() CommandRunner {
	if m.Runner == nil {
		return ExecRunner{}
	}
	return m.Runner
}

func (m *BIRDProcessManager) starter() ProcessStarter {
	if m.Starter == nil {
		return ExecStarter{}
	}
	return m.Starter
}

func (s Server) birdSocketPath() string {
	if s.BIRDSocketPath == "" {
		return DefaultBIRDSocketPath
	}
	return s.BIRDSocketPath
}
