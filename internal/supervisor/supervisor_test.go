package supervisor

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recordingRunner struct {
	name   string
	args   []string
	calls  []string
	output string
	err    error
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	r.calls = append(r.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	return r.output, r.err
}

type recordingStarter struct {
	name    string
	args    []string
	process *recordingProcess
	err     error
}

func (s *recordingStarter) StartProcess(name string, args ...string) (Process, error) {
	s.name = name
	s.args = append([]string(nil), args...)
	if s.err != nil {
		return nil, s.err
	}
	s.process = &recordingProcess{}
	return s.process, nil
}

type recordingProcess struct {
	killed bool
}

func (p *recordingProcess) Kill() error {
	p.killed = true
	return nil
}

func (p *recordingProcess) Wait() error {
	return nil
}

type fakeWGManager struct {
	started   bool
	stopped   bool
	restarted bool
}

func (m *fakeWGManager) Start(ctx context.Context) (string, error) {
	m.started = true
	return "started\n", nil
}

func (m *fakeWGManager) Stop(ctx context.Context) (string, error) {
	m.stopped = true
	return "stopped\n", nil
}

func (m *fakeWGManager) Restart(ctx context.Context) (string, error) {
	m.restarted = true
	return "restarted\n", nil
}

func TestSupervisorPing(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "supervisor.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- (Server{SocketPath: socketPath}).Serve(ctx)
	}()
	client := Client{SocketPath: socketPath, Timeout: time.Second}
	var resp Response
	var err error
	for i := 0; i < 50; i++ {
		resp, err = client.Call(context.Background(), ActionPing)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK || resp.Output != "pong" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop")
	}
}

func TestSupervisorRejectsUnknownAction(t *testing.T) {
	resp := (Server{}).dispatch(Request{Action: "shell"})
	if resp.OK || resp.Error == "" {
		t.Fatalf("expected rejected action, got %#v", resp)
	}
}

func TestSupervisorBIRDReloadRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "configured\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDReload})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "birdc" || strings.Join(runner.args, " ") != "-s /run/bird/bird.ctl configure" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
	if resp.Output != "configured\n" {
		t.Fatalf("unexpected output: %q", resp.Output)
	}
}

func TestSupervisorBIRDStartRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "started\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDStart})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "bird" || strings.Join(runner.args, " ") != "-c /app-state/bird/bird.conf -s /run/bird/bird.ctl" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
}

func TestSupervisorBIRDStatusRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "peplink BGP up\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDStatus})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "birdc" || strings.Join(runner.args, " ") != "-s /run/bird/bird.ctl show protocols" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
}

func TestSupervisorWGStatusRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "interface: wg0\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionWGStatus})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "wg" || strings.Join(runner.args, " ") != "show" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
}

func TestSupervisorWGLifecycleDispatchesToManager(t *testing.T) {
	manager := &fakeWGManager{}
	server := Server{WGManager: manager}
	for _, action := range []string{ActionWGStart, ActionWGStop, ActionWGRestart} {
		resp := server.dispatch(Request{Action: action})
		if !resp.OK {
			t.Fatalf("%s failed: %#v", action, resp)
		}
	}
	if !manager.started || !manager.stopped || !manager.restarted {
		t.Fatalf("manager calls not recorded: %#v", manager)
	}
}

func TestWGProcessManagerStartRunsExpectedSequence(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "wg0.conf")
	if err := os.WriteFile(configPath, []byte(`[Interface]
PrivateKey = private-value
Address = 10.0.15.7/32

[Peer]
PublicKey = abcdefghijklmnopqrstuvwxyz1234567890
Endpoint = 172.17.62.1:51820
AllowedIPs = 0.0.0.0/0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{output: "ok"}
	starter := &recordingStarter{}
	manager := &WGProcessManager{
		ConfigPath: configPath,
		Runner:     runner,
		Starter:    starter,
	}
	out, err := manager.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if starter.name != "wireguard-go" || strings.Join(starter.args, " ") != "wg0" {
		t.Fatalf("unexpected start command: %s %#v", starter.name, starter.args)
	}
	want := []string{
		"wg setconf wg0 " + configPath,
		"ip addr replace 10.0.15.7/32 dev wg0",
		"ip link set mtu 1320 up dev wg0",
	}
	if strings.Join(runner.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	if out == "" {
		t.Fatal("expected command output")
	}
}

func TestWGProcessManagerStartRejectsInvalidConfig(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "wg0.conf")
	if err := os.WriteFile(configPath, []byte("not wg"), 0o600); err != nil {
		t.Fatal(err)
	}
	starter := &recordingStarter{}
	manager := &WGProcessManager{ConfigPath: configPath, Starter: starter}
	if _, err := manager.Start(context.Background()); err == nil {
		t.Fatal("expected invalid config error")
	}
	if starter.name != "" {
		t.Fatalf("wireguard-go should not start for invalid config: %s", starter.name)
	}
}

func TestWGProcessManagerStopDeletesInterfaceAndKillsProcess(t *testing.T) {
	runner := &recordingRunner{output: "deleted"}
	process := &recordingProcess{}
	manager := &WGProcessManager{Runner: runner}
	manager.process = process
	out, err := manager.Stop(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(runner.calls, " ") != "ip link delete dev wg0" {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	if !process.killed {
		t.Fatal("expected process to be killed")
	}
	if out == "" {
		t.Fatal("expected command output")
	}
}

func TestSupervisorBIRDReloadReportsCommandFailure(t *testing.T) {
	runner := &recordingRunner{output: "socket failed\n", err: errors.New("exit status 1")}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDReload})
	if resp.OK || resp.Output != "socket failed\n" || resp.Error == "" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
