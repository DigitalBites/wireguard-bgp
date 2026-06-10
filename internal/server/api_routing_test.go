package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"peplink-wg-bgp/internal/config"
	"peplink-wg-bgp/internal/orchestrator"
	"peplink-wg-bgp/internal/supervisor"
)

func TestRoutingApplyRequiresCSRF(t *testing.T) {
	srv := newTestServer(t, configDefaultForRoutingTest())
	req := httptest.NewRequest(http.MethodPost, "/api/routing/apply", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRoutingApplyOrchestratesSupervisorActions(t *testing.T) {
	manager := &routingWGManagerTestDouble{
		restartOutput: "wg restarted\n",
	}
	routeManager := &routingRouteManagerTestDouble{output: "routes applied\n"}
	runner, srv, stop := newSupervisorBackedTestServerWithManagers(t, manager, routeManager)
	runner.outputs = map[string]string{"wg show": "interface: wg0\n"}
	defer stop()
	req := httptest.NewRequest(http.MethodPost, "/api/routing/apply", nil)
	req.RemoteAddr = "192.0.2.55:49152"
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var result orchestrator.Result
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("unexpected result: %#v", result)
	}
	gotActions := routingResultActions(result)
	wantActions := []string{supervisor.ActionWGRestart, supervisor.ActionRoutesApply, supervisor.ActionBIRDStart, supervisor.ActionBIRDReload}
	if !reflect.DeepEqual(gotActions, wantActions) {
		t.Fatalf("actions=%#v", gotActions)
	}
	wantCommands := []string{
		"wg show",
		"birdc -s /run/bird/bird.ctl show status",
		"birdc -s /run/bird/bird.ctl configure",
	}
	if !reflect.DeepEqual(runner.commandLines(), wantCommands) {
		t.Fatalf("commands=%#v", runner.commandLines())
	}
	if !manager.restarted {
		t.Fatal("expected WireGuard restart")
	}
	if !routeManager.applied {
		t.Fatal("expected routes apply")
	}
	if routeManager.pinned != "" {
		t.Fatalf("expected client route pin to be disabled by default, got %q", routeManager.pinned)
	}
}

func TestRoutingApplyPinsClientWhenEnabled(t *testing.T) {
	manager := &routingWGManagerTestDouble{restartOutput: "wg restarted\n"}
	routeManager := &routingRouteManagerTestDouble{output: "routes applied\n"}
	_, srv, stop := newSupervisorBackedTestServerWithManagers(t, manager, routeManager)
	defer stop()
	srv.cfg.Runtime.PinDashboardClientRoute = true
	req := httptest.NewRequest(http.MethodPost, "/api/routing/apply", nil)
	req.RemoteAddr = "192.0.2.55:49152"
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if routeManager.pinned != "192.0.2.55" {
		t.Fatalf("expected client route pin, got %q", routeManager.pinned)
	}
}

func TestRoutingApplyStartsWireGuardWhenStopped(t *testing.T) {
	manager := &routingWGManagerTestDouble{startOutput: "wg started\n"}
	routeManager := &routingRouteManagerTestDouble{output: "routes applied\n"}
	runner, srv, stop := newSupervisorBackedTestServerWithManagers(t, manager, routeManager)
	runner.outputs = map[string]string{"wg show": ""}
	defer stop()
	req := httptest.NewRequest(http.MethodPost, "/api/routing/apply", nil)
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !manager.started || manager.restarted {
		t.Fatalf("expected start without restart: %#v", manager)
	}
	if strings.Contains(rec.Body.String(), "wireguard interface already absent") {
		t.Fatalf("unexpected restart output: %s", rec.Body.String())
	}
}

func TestRoutingStopOrchestratesShutdownOrder(t *testing.T) {
	manager := &routingWGManagerTestDouble{
		stopOutput: "wg stopped\n",
	}
	runner, srv, stop := newSupervisorBackedTestServerWithWG(t, manager)
	defer stop()
	req := httptest.NewRequest(http.MethodPost, "/api/routing/stop", nil)
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"action":"bird.stop"`) || !strings.Contains(rec.Body.String(), `"action":"wg.stop"`) {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	wantCommands := []string{"birdc -s /run/bird/bird.ctl down"}
	if !reflect.DeepEqual(runner.commandLines(), wantCommands) {
		t.Fatalf("commands=%#v", runner.commandLines())
	}
	if !manager.stopped {
		t.Fatal("expected WireGuard stop")
	}
}

func configDefaultForRoutingTest() config.App {
	return config.Default()
}

func routingResultActions(result orchestrator.Result) []string {
	actions := make([]string, 0, len(result.Steps))
	for _, step := range result.Steps {
		actions = append(actions, step.Action)
	}
	return actions
}

type routingWGManagerTestDouble struct {
	startOutput   string
	stopOutput    string
	restartOutput string
	started       bool
	stopped       bool
	restarted     bool
}

type routingRouteManagerTestDouble struct {
	output  string
	applied bool
	pinned  string
}

func (m *routingRouteManagerTestDouble) Apply(ctx context.Context) (string, error) {
	m.applied = true
	return m.output, nil
}

func (m *routingRouteManagerTestDouble) PinClient(ctx context.Context, clientIP string) (string, error) {
	m.pinned = clientIP
	return "client pinned\n", nil
}

func (m *routingWGManagerTestDouble) Start(ctx context.Context) (string, error) {
	m.started = true
	return m.startOutput, nil
}

func (m *routingWGManagerTestDouble) Stop(ctx context.Context) (string, error) {
	m.stopped = true
	return m.stopOutput, nil
}

func (m *routingWGManagerTestDouble) Restart(ctx context.Context) (string, error) {
	m.restarted = true
	return m.restartOutput, nil
}
