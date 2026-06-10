package supervisor

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peplink-wg-bgp/internal/config"
)

type recordingRunner struct {
	name    string
	args    []string
	calls   []string
	output  string
	err     error
	outputs map[string]string
	errs    map[string]error
}

func (r *recordingRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	commandLine := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.calls = append(r.calls, commandLine)
	if r.outputs != nil {
		if output, ok := r.outputs[commandLine]; ok {
			return output, r.errs[commandLine]
		}
	}
	if r.errs != nil {
		if err, ok := r.errs[commandLine]; ok {
			return r.output, err
		}
	}
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

type birdStartRunner struct {
	calls []string
}

func (r *birdStartRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	commandLine := strings.TrimSpace(name + " " + strings.Join(args, " "))
	r.calls = append(r.calls, commandLine)
	if len(r.calls) == 1 {
		return "", errors.New("socket down")
	}
	return "BIRD ready\n", nil
}

type fakeResolver struct {
	addrs map[string][]net.IPAddr
	err   error
}

func (r fakeResolver) LookupIPAddr(ctx context.Context, host string) ([]net.IPAddr, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.addrs[host], nil
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

type fakeRouteManager struct {
	applied bool
	pinned  string
}

func (m *fakeRouteManager) Apply(ctx context.Context) (string, error) {
	m.applied = true
	return "routes applied\n", nil
}

func (m *fakeRouteManager) PinClient(ctx context.Context, clientIP string) (string, error) {
	m.pinned = clientIP
	return "client pinned\n", nil
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

func TestSupervisorAllowsConfiguredPeerUID(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "supervisor.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- (Server{SocketPath: socketPath, AllowedUID: os.Getuid()}).Serve(ctx)
	}()
	waitForSocket(t, socketPath)
	resp, err := (Client{SocketPath: socketPath, Timeout: time.Second}).Call(context.Background(), ActionPing)
	if err != nil {
		t.Fatal(err)
	}
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	cancel()
	assertServerStopped(t, errCh)
}

func TestSupervisorRejectsUnexpectedPeerUID(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "supervisor.sock")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- (Server{SocketPath: socketPath, AllowedUID: os.Getuid() + 1}).Serve(ctx)
	}()
	waitForSocket(t, socketPath)
	_, err := (Client{SocketPath: socketPath, Timeout: time.Second}).Call(context.Background(), ActionPing)
	if err == nil || !strings.Contains(err.Error(), "unauthorized supervisor peer uid") {
		t.Fatalf("expected unauthorized peer error, got %v", err)
	}
	cancel()
	assertServerStopped(t, errCh)
}

func TestSupervisorRejectsUnknownAction(t *testing.T) {
	resp := (Server{}).dispatch(Request{Action: "shell"})
	if resp.OK || resp.Error == "" {
		t.Fatalf("expected rejected action, got %#v", resp)
	}
}

func waitForSocket(t *testing.T, socketPath string) {
	t.Helper()
	for i := 0; i < 50; i++ {
		info, err := os.Stat(socketPath)
		if err == nil && info.Mode().Type() == os.ModeSocket {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("supervisor socket %s was not created", socketPath)
}

func assertServerStopped(t *testing.T, errCh <-chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor did not stop")
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

func TestSupervisorBIRDStartReturnsWhenAlreadyRunning(t *testing.T) {
	runner := &recordingRunner{output: "BIRD ready\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDStart})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "birdc" || strings.Join(runner.args, " ") != "-s /run/bird/bird.ctl show status" {
		t.Fatalf("unexpected command: %s %#v", runner.name, runner.args)
	}
	if !strings.Contains(resp.Output, "bird already running") {
		t.Fatalf("unexpected output: %q", resp.Output)
	}
}

func TestBIRDProcessManagerStartStartsProcessAndPollsStatus(t *testing.T) {
	runner := &birdStartRunner{}
	starter := &recordingStarter{}
	manager := &BIRDProcessManager{Runner: runner, Starter: starter}
	out, err := manager.Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if starter.name != "bird" || strings.Join(starter.args, " ") != "-c /app-state/bird/bird.conf -s /run/bird/bird.ctl" {
		t.Fatalf("unexpected start command: %s %#v", starter.name, starter.args)
	}
	wantCalls := []string{
		"birdc -s /run/bird/bird.ctl show status",
		"birdc -s /run/bird/bird.ctl show status",
	}
	if strings.Join(runner.calls, "|") != strings.Join(wantCalls, "|") {
		t.Fatalf("unexpected status calls: %#v", runner.calls)
	}
	if out != "BIRD ready\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestSupervisorBIRDStopRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "stopped\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDStop})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "birdc" || strings.Join(runner.args, " ") != "-s /run/bird/bird.ctl down" {
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

func TestSupervisorBIRDDetailsRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "BGP state: Established\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDDetails})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "birdc" || strings.Join(runner.args, " ") != "-s /run/bird/bird.ctl show protocols all peplink" {
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

func TestSupervisorWGDumpRunsFixedCommand(t *testing.T) {
	runner := &recordingRunner{output: "dump\n"}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionWGDump})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if runner.name != "wg" || strings.Join(runner.args, " ") != "show wg0 dump" {
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

func TestSupervisorRoutesApplyDispatchesToManager(t *testing.T) {
	manager := &fakeRouteManager{}
	resp := (Server{RouteManager: manager}).dispatch(Request{Action: ActionRoutesApply})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if !manager.applied {
		t.Fatal("expected route manager to be called")
	}
}

func TestSupervisorRoutesPinClientDispatchesToManager(t *testing.T) {
	manager := &fakeRouteManager{}
	resp := (Server{RouteManager: manager}).dispatch(Request{
		Action: ActionRoutesPinClient,
		Params: map[string]string{"clientIP": "192.0.2.55"},
	})
	if !resp.OK {
		t.Fatalf("unexpected response: %#v", resp)
	}
	if manager.pinned != "192.0.2.55" {
		t.Fatalf("expected pinned client IP, got %q", manager.pinned)
	}
}

func TestWGProcessManagerStartRunsExpectedSequence(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "wg0.conf")
	if err := os.WriteFile(configPath, []byte(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Address = 10.0.15.7/32
MTU = 1280

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
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
	if starter.name != "" {
		t.Fatalf("wireguard-go should not start when interface exists: %s %#v", starter.name, starter.args)
	}
	want := []string{
		"ip link show dev wg0",
		"ip addr replace 10.0.15.7/32 dev wg0",
		"ip link set mtu 1280 up dev wg0",
	}
	if len(runner.calls) != 4 || !strings.HasPrefix(runner.calls[1], "wg setconf wg0 /tmp/peplink-wg-setconf-") {
		t.Fatalf("unexpected setconf call: %#v", runner.calls)
	}
	got := []string{runner.calls[0], runner.calls[2], runner.calls[3]}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
	if out == "" {
		t.Fatal("expected command output")
	}
}

func TestWGProcessManagerStartCreatesKernelInterfaceWhenMissing(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "wg0.conf")
	if err := os.WriteFile(configPath, []byte(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Endpoint = 172.17.62.1:51820
AllowedIPs = 0.0.0.0/0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{
		errs: map[string]error{
			"ip link show dev wg0": errors.New("missing"),
		},
	}
	starter := &recordingStarter{}
	manager := &WGProcessManager{ConfigPath: configPath, Runner: runner, Starter: starter}
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if starter.name != "" {
		t.Fatalf("wireguard-go should not start when kernel interface creation works: %s", starter.name)
	}
	wantPrefix := []string{
		"ip link show dev wg0",
		"ip link add dev wg0 type wireguard",
	}
	if strings.Join(runner.calls[:2], "|") != strings.Join(wantPrefix, "|") {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
}

func TestWGProcessManagerStartFallsBackToWireguardGo(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "wg0.conf")
	if err := os.WriteFile(configPath, []byte(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Endpoint = 172.17.62.1:51820
AllowedIPs = 0.0.0.0/0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{
		errs: map[string]error{
			"ip link show dev wg0":               errors.New("missing"),
			"ip link add dev wg0 type wireguard": errors.New("not supported"),
		},
	}
	starter := &recordingStarter{}
	manager := &WGProcessManager{ConfigPath: configPath, Runner: runner, Starter: starter}
	if _, err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if starter.name != "wireguard-go" || strings.Join(starter.args, " ") != "wg0" {
		t.Fatalf("unexpected fallback start command: %s %#v", starter.name, starter.args)
	}
}

func TestRouteApplyManagerRunsExpectedSequence(t *testing.T) {
	dir := t.TempDir()
	appPath := filepath.Join(dir, "app.yaml")
	wgPath := filepath.Join(dir, "wg0.conf")
	cfg := config.Default()
	cfg.ConfigDir = dir
	cfg.BIRDConfigPath = filepath.Join(dir, "bird", "bird.conf")
	cfg.BIRD.LocalASN = 65060
	cfg.BIRD.PeerASN = 65001
	cfg.BIRD.PeerIP = "192.168.50.1"
	if err := config.Save(appPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wgPath, []byte(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Endpoint = 172.17.62.1:51820
AllowedIPs = 0.0.0.0/0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{output: "default via 172.32.0.1 dev eth0\n"}
	manager := &RouteApplyManager{
		AppConfigPath: appPath,
		WGConfigPath:  wgPath,
		Runner:        runner,
	}
	if _, err := manager.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ip route show default dev eth0",
		"ip route replace 172.17.62.1/32 via 172.32.0.1 dev eth0",
		"ip route replace 0.0.0.0/1 dev wg0",
		"ip route replace 128.0.0.0/1 dev wg0",
	}
	if strings.Join(runner.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
}

func TestRouteApplyManagerAppliesPersistedPinnedClientRoutes(t *testing.T) {
	dir := t.TempDir()
	appPath := filepath.Join(dir, "app.yaml")
	wgPath := filepath.Join(dir, "wg0.conf")
	cfg := config.Default()
	cfg.ConfigDir = dir
	cfg.BIRDConfigPath = filepath.Join(dir, "bird", "bird.conf")
	cfg.Runtime.PinnedClientRoutes = []string{"192.168.64.1/32"}
	cfg.BIRD.LocalASN = 65060
	cfg.BIRD.PeerASN = 65001
	cfg.BIRD.PeerIP = "192.168.50.1"
	if err := config.Save(appPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wgPath, []byte(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Endpoint = 172.17.62.1:51820
AllowedIPs = 0.0.0.0/0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{output: "default via 172.32.0.1 dev eth0\n"}
	manager := &RouteApplyManager{AppConfigPath: appPath, WGConfigPath: wgPath, Runner: runner}
	if _, err := manager.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ip route show default dev eth0",
		"ip route replace 172.17.62.1/32 via 172.32.0.1 dev eth0",
		"ip route show default dev eth0",
		"ip route replace 192.168.64.1/32 via 172.32.0.1 dev eth0",
		"ip route replace 0.0.0.0/1 dev wg0",
		"ip route replace 128.0.0.0/1 dev wg0",
	}
	if strings.Join(runner.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
}

func TestRouteApplyManagerPinsClientViaDefaultInterface(t *testing.T) {
	dir := t.TempDir()
	appPath := filepath.Join(dir, "app.yaml")
	cfg := config.Default()
	cfg.ConfigDir = dir
	cfg.BIRDConfigPath = filepath.Join(dir, "bird", "bird.conf")
	if err := config.Save(appPath, cfg); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{output: "default via 172.32.0.1 dev eth0\n"}
	manager := &RouteApplyManager{
		AppConfigPath: appPath,
		Runner:        runner,
	}
	if _, err := manager.PinClient(context.Background(), "192.0.2.55"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"ip route show default dev eth0",
		"ip route replace 192.0.2.55/32 via 172.32.0.1 dev eth0",
	}
	if strings.Join(runner.calls, "|") != strings.Join(want, "|") {
		t.Fatalf("unexpected calls: %#v", runner.calls)
	}
}

func TestRouteApplyManagerResolvesHostnameEndpoint(t *testing.T) {
	dir := t.TempDir()
	appPath := filepath.Join(dir, "app.yaml")
	wgPath := filepath.Join(dir, "wg0.conf")
	cfg := config.Default()
	cfg.ConfigDir = dir
	cfg.BIRDConfigPath = filepath.Join(dir, "bird", "bird.conf")
	cfg.BIRD.LocalASN = 65060
	cfg.BIRD.PeerASN = 65001
	cfg.BIRD.PeerIP = "192.168.50.1"
	if err := config.Save(appPath, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(wgPath, []byte(`[Interface]
PrivateKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=

[Peer]
PublicKey = AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=
Endpoint = vpn.example.test:51820
AllowedIPs = 0.0.0.0/0
`), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{output: "default via 172.32.0.1 dev eth0\n"}
	manager := &RouteApplyManager{
		AppConfigPath: appPath,
		WGConfigPath:  wgPath,
		Runner:        runner,
		Resolver: fakeResolver{addrs: map[string][]net.IPAddr{
			"vpn.example.test": {{IP: net.ParseIP("172.17.62.1")}},
		}},
	}
	if _, err := manager.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantPrefix := []string{
		"ip route show default dev eth0",
		"ip route replace 172.17.62.1/32 via 172.32.0.1 dev eth0",
	}
	if strings.Join(runner.calls[:2], "|") != strings.Join(wantPrefix, "|") {
		t.Fatalf("unexpected calls: %#v", runner.calls)
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

func TestWGProcessManagerStopSuppressesExpectedMissingInterfaceOutput(t *testing.T) {
	runner := &recordingRunner{
		output: `Cannot find device "wg0"`,
		err:    errors.New("exit status 1"),
	}
	manager := &WGProcessManager{Runner: runner}
	out, err := manager.Stop(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out, "Cannot find device") {
		t.Fatalf("expected missing interface output to be suppressed, got %q", out)
	}
	if out != "wireguard interface already absent\n" {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestSupervisorBIRDReloadReportsCommandFailure(t *testing.T) {
	runner := &recordingRunner{output: "socket failed\n", err: errors.New("exit status 1")}
	resp := (Server{Runner: runner}).dispatch(Request{Action: ActionBIRDReload})
	if resp.OK || resp.Output != "socket failed\n" || resp.Error == "" {
		t.Fatalf("unexpected response: %#v", resp)
	}
}
