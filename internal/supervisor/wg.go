package supervisor

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"peplink-wg-bgp/internal/wg"
)

const (
	DefaultWGConfigPath = "/app-state/wireguard/wg0.conf"
	DefaultWGInterface  = "wg0"
	DefaultWGMTU        = 1320
	DefaultWGUAPIDir    = "/run/wireguard"
)

const (
	defaultWGReadyTimeout = 3 * time.Second
	defaultWGReadyPoll    = 100 * time.Millisecond
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
	UAPIDir    string
	ReadyAfter time.Duration
	ReadyEvery time.Duration

	mu      sync.Mutex
	process Process
}

func (s Server) statusWG() Response {
	return s.runCommand(ActionWGStatus, "wg", "show")
}

func (s Server) dumpWG() Response {
	return s.runCommand(ActionWGDump, "wg", "show", DefaultWGInterface, "dump")
}

func (s Server) startWG() Response {
	ctx, cancel := s.commandContext()
	defer cancel()
	out, err := s.wgManager().Start(ctx)
	return wgResponse(ActionWGStart, out, err)
}

func (s Server) stopWG() Response {
	ctx, cancel := s.commandContext()
	defer cancel()
	out, err := s.wgManager().Stop(ctx)
	return wgResponse(ActionWGStop, out, err)
}

func (s Server) restartWG() Response {
	ctx, cancel := s.commandContext()
	defer cancel()
	out, err := s.wgManager().Restart(ctx)
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

	var out bytes.Buffer
	if err := m.ensureInterface(ctx, &out); err != nil {
		return out.String(), err
	}
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
	if err := m.runStep(ctx, &out, "ip", "link", "set", "mtu", fmt.Sprint(m.effectiveMTU(meta)), "up", "dev", m.iface()); err != nil {
		_ = m.killLocked()
		return out.String(), err
	}
	return out.String(), nil
}

func (m *WGProcessManager) Stop(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out bytes.Buffer
	deleteOut, deleteErr := m.runner().Run(ctx, "ip", "link", "delete", "dev", m.iface())
	if deleteOut != "" && !isMissingInterfaceOutput(deleteOut) {
		appendOutput(&out, deleteOut)
	}
	killErr := m.killLocked()
	if killErr != nil {
		return out.String(), killErr
	}
	if deleteErr != nil && !isMissingInterfaceOutput(deleteOut) {
		return out.String(), deleteErr
	}
	if deleteErr != nil {
		out.WriteString("wireguard interface already absent\n")
	}
	return out.String(), nil
}

func (m *WGProcessManager) ensureInterface(ctx context.Context, out *bytes.Buffer) error {
	showOut, showErr := m.runner().Run(ctx, "ip", "link", "show", "dev", m.iface())
	if showErr == nil {
		appendOutput(out, showOut)
		out.WriteString("wireguard interface already exists\n")
		return nil
	}
	if showOut != "" && !isMissingInterfaceOutput(showOut) {
		appendOutput(out, showOut)
	}
	addOut, addErr := m.runner().Run(ctx, "ip", "link", "add", "dev", m.iface(), "type", "wireguard")
	if addErr == nil {
		appendOutput(out, addOut)
		return nil
	}
	appendOutput(out, addOut)
	if m.process != nil {
		return nil
	}
	starter := m.starter()
	proc, err := starter.StartProcess("wireguard-go", m.iface())
	if err != nil {
		return fmt.Errorf("create WireGuard interface with kernel or wireguard-go: %w", err)
	}
	m.process = proc
	if err := m.waitForUserspaceInterface(ctx); err != nil {
		_ = m.killLocked()
		return err
	}
	out.WriteString("wireguard-go interface ready\n")
	return nil
}

func (m *WGProcessManager) waitForUserspaceInterface(ctx context.Context) error {
	deadline := time.NewTimer(m.readyTimeout())
	defer deadline.Stop()
	ticker := time.NewTicker(m.readyPoll())
	defer ticker.Stop()

	for {
		if m.userspaceInterfaceReady(ctx) {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("wireguard-go did not create %s before timeout", m.uapiSocketPath())
		case <-ticker.C:
		}
	}
}

func (m *WGProcessManager) userspaceInterfaceReady(ctx context.Context) bool {
	if _, err := os.Stat(m.uapiSocketPath()); err != nil {
		return false
	}
	_, err := m.runner().Run(ctx, "ip", "link", "show", "dev", m.iface())
	return err == nil
}

func isMissingInterfaceOutput(output string) bool {
	lower := strings.ToLower(output)
	return strings.Contains(lower, "cannot find device") ||
		strings.Contains(lower, "does not exist") ||
		strings.Contains(lower, "no such device")
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
	appendOutput(out, stepOut)
	if err != nil {
		return err
	}
	return nil
}

func appendOutput(out *bytes.Buffer, text string) {
	if text == "" {
		return
	}
	out.WriteString(text)
	if !strings.HasSuffix(text, "\n") {
		out.WriteByte('\n')
	}
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

func (m *WGProcessManager) effectiveMTU(meta wg.ConfigMeta) int {
	if meta.InterfaceMTU != 0 {
		return meta.InterfaceMTU
	}
	return m.mtu()
}

func (m *WGProcessManager) runner() CommandRunner {
	if m.Runner == nil {
		return ExecRunner{}
	}
	return m.Runner
}

func (m *WGProcessManager) uapiSocketPath() string {
	dir := m.UAPIDir
	if dir == "" {
		dir = DefaultWGUAPIDir
	}
	return filepath.Join(dir, m.iface()+".sock")
}

func (m *WGProcessManager) readyTimeout() time.Duration {
	if m.ReadyAfter == 0 {
		return defaultWGReadyTimeout
	}
	return m.ReadyAfter
}

func (m *WGProcessManager) readyPoll() time.Duration {
	if m.ReadyEvery == 0 {
		return defaultWGReadyPoll
	}
	return m.ReadyEvery
}

func (m *WGProcessManager) starter() ProcessStarter {
	if m.Starter == nil {
		return ExecStarter{}
	}
	return m.Starter
}
