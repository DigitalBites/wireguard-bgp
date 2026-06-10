package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"peplink-wg-bgp/internal/wg"
)

const (
	DefaultWGConfigPath = "/app-state/wireguard/wg0.conf"
	DefaultWGInterface  = "wg0"
	DefaultWGMTU        = 1320
)

type WGManager interface {
	Start(ctx context.Context) (string, error)
	Stop(ctx context.Context) (string, error)
	Restart(ctx context.Context) (string, error)
}

type ProcessStarter interface {
	StartProcess(name string, args ...string) (Process, error)
}

type Process interface {
	Kill() error
	Wait() error
}

type ExecStarter struct{}

type execProcess struct {
	cmd *exec.Cmd
}

type WGProcessManager struct {
	ConfigPath string
	Interface  string
	MTU        int
	Runner     CommandRunner
	Starter    ProcessStarter

	mu      sync.Mutex
	process Process
}

func (s Server) statusWG() Response {
	return s.runCommand(ActionWGStatus, "wg", "show")
}

func (s Server) startWG() Response {
	out, err := s.wgManager().Start(context.Background())
	return wgResponse(ActionWGStart, out, err)
}

func (s Server) stopWG() Response {
	out, err := s.wgManager().Stop(context.Background())
	return wgResponse(ActionWGStop, out, err)
}

func (s Server) restartWG() Response {
	out, err := s.wgManager().Restart(context.Background())
	return wgResponse(ActionWGRestart, out, err)
}

func (s Server) wgManager() WGManager {
	if s.WGManager != nil {
		return s.WGManager
	}
	return &WGProcessManager{Runner: s.Runner}
}

func (ExecStarter) StartProcess(name string, args ...string) (Process, error) {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	proc := &execProcess{cmd: cmd}
	go func() {
		_ = proc.Wait()
	}()
	return proc, nil
}

func (p *execProcess) Kill() error {
	if p.cmd.Process == nil {
		return nil
	}
	return p.cmd.Process.Kill()
}

func (p *execProcess) Wait() error {
	return p.cmd.Wait()
}

func (m *WGProcessManager) Start(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.process != nil {
		return "wireguard-go already running\n", nil
	}
	cfgPath := m.configPath()
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return "", fmt.Errorf("read WireGuard config: %w", err)
	}
	meta, err := wg.ValidateConfig(string(data))
	if err != nil {
		return "", err
	}
	setconfPath, cleanup, err := writeSetconfFile(string(data))
	if err != nil {
		return "", err
	}
	defer cleanup()

	starter := m.starter()
	proc, err := starter.StartProcess("wireguard-go", m.iface())
	if err != nil {
		return "", fmt.Errorf("start wireguard-go: %w", err)
	}
	m.process = proc

	var out bytes.Buffer
	if err := m.runStep(ctx, &out, "wg", "setconf", m.iface(), setconfPath); err != nil {
		_ = m.killLocked()
		return out.String(), err
	}
	if meta.InterfaceAddress != "" {
		if err := m.runStep(ctx, &out, "ip", "addr", "replace", meta.InterfaceAddress, "dev", m.iface()); err != nil {
			_ = m.killLocked()
			return out.String(), err
		}
	}
	if err := m.runStep(ctx, &out, "ip", "link", "set", "mtu", fmt.Sprint(m.mtu()), "up", "dev", m.iface()); err != nil {
		_ = m.killLocked()
		return out.String(), err
	}
	return out.String(), nil
}

func (m *WGProcessManager) Stop(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out bytes.Buffer
	if err := m.runStep(ctx, &out, "ip", "link", "delete", "dev", m.iface()); err != nil {
		return out.String(), err
	}
	if err := m.killLocked(); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

func writeSetconfFile(input string) (string, func(), error) {
	file, err := os.CreateTemp("", "peplink-wg-setconf-*.conf")
	if err != nil {
		return "", func() {}, fmt.Errorf("create stripped WireGuard config: %w", err)
	}
	cleanup := func() {
		_ = os.Remove(file.Name())
	}
	if _, err := file.WriteString(wg.SetconfConfig(input)); err != nil {
		_ = file.Close()
		cleanup()
		return "", func() {}, fmt.Errorf("write stripped WireGuard config: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return "", func() {}, fmt.Errorf("close stripped WireGuard config: %w", err)
	}
	return file.Name(), cleanup, nil
}

func (m *WGProcessManager) Restart(ctx context.Context) (string, error) {
	stopOut, stopErr := m.Stop(ctx)
	startOut, startErr := m.Start(ctx)
	out := stopOut + startOut
	if stopErr != nil {
		return out, stopErr
	}
	if startErr != nil {
		return out, startErr
	}
	return out, nil
}

func wgResponse(action string, out string, err error) Response {
	if err != nil {
		return Response{OK: false, Action: action, Output: out, Error: err.Error()}
	}
	return Response{OK: true, Action: action, Output: out}
}

func (m *WGProcessManager) runStep(ctx context.Context, out *bytes.Buffer, name string, args ...string) error {
	stepOut, err := m.runner().Run(ctx, name, args...)
	if stepOut != "" {
		out.WriteString(stepOut)
		if !strings.HasSuffix(stepOut, "\n") {
			out.WriteByte('\n')
		}
	}
	if err != nil {
		return err
	}
	return nil
}

func (m *WGProcessManager) killLocked() error {
	if m.process == nil {
		return nil
	}
	err := m.process.Kill()
	m.process = nil
	return err
}

func (m *WGProcessManager) configPath() string {
	if m.ConfigPath == "" {
		return DefaultWGConfigPath
	}
	return m.ConfigPath
}

func (m *WGProcessManager) iface() string {
	if m.Interface == "" {
		return DefaultWGInterface
	}
	return m.Interface
}

func (m *WGProcessManager) mtu() int {
	if m.MTU == 0 {
		return DefaultWGMTU
	}
	return m.MTU
}

func (m *WGProcessManager) runner() CommandRunner {
	if m.Runner == nil {
		return ExecRunner{}
	}
	return m.Runner
}

func (m *WGProcessManager) starter() ProcessStarter {
	if m.Starter == nil {
		return ExecStarter{}
	}
	return m.Starter
}
