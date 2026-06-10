package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"peplink-wg-bgp/internal/config"
)

func TestStatusEndpoint(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"wireGuard"`) {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestStatusEndpointReportsRuntimeState(t *testing.T) {
	runner, srv, stop := newSupervisorBackedTestServer(t)
	defer stop()
	runner.outputs = map[string]string{
		"wg show":          "interface: wg0\n  public key: abc\n",
		"wg show wg0 dump": "priv\tpub\t51820\toff\npeer\tpsk\t172.17.62.1:51820\t0.0.0.0/0\t0\t1234\t5678\t25\n",
		"birdc -s /run/bird/bird.ctl show protocols":             "peplink BGP master up 2026-06-10 Established\n",
		"birdc -s /run/bird/bird.ctl show protocols all peplink": "Name       Proto      Table      State  Since         Info\npeplink    BGP        master4    up     2026-06-10    Established\n  BGP state:          Established\n  Neighbor address:   192.168.50.1\n  Neighbor AS:        65001\n  Local AS:           65060\n  Routes:             2 imported, 0 exported\n",
	}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body runtimeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.WireGuard.State != "up" || body.BIRD.State != "up" {
		t.Fatalf("unexpected states: %#v", body)
	}
	if body.WireGuard.Stats["endpoint"] != "172.17.62.1:51820" || body.WireGuard.Stats["rxBytes"] != "1234" || body.WireGuard.Stats["latestHandshake"] != "never" {
		t.Fatalf("unexpected wireguard stats: %#v", body.WireGuard.Stats)
	}
	if body.BIRD.Stats["bgpState"] != "Established" || body.BIRD.Stats["neighbor"] != "192.168.50.1" {
		t.Fatalf("unexpected bird stats: %#v", body.BIRD.Stats)
	}
	wantCommands := []string{
		"wg show",
		"wg show wg0 dump",
		"birdc -s /run/bird/bird.ctl show protocols",
		"birdc -s /run/bird/bird.ctl show protocols all peplink",
	}
	if !reflect.DeepEqual(runner.commandLines(), wantCommands) {
		t.Fatalf("commands=%#v", runner.commandLines())
	}
}

func TestStatusEndpointReportsUnavailableWhenSupervisorDown(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body runtimeStatus
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.WireGuard.State != "unavailable" || body.BIRD.State != "unavailable" {
		t.Fatalf("unexpected states: %#v", body)
	}
}

func TestDashboardIncludesRoutingControls(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{
		`name="csrf-token"`,
		`data-routing-action="apply"`,
		`data-routing-action="restart"`,
		`data-routing-action="stop"`,
		`id="routing-steps"`,
		`data-refresh-indicator`,
		`data-refresh-age`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("dashboard missing %q:\n%s", want, rec.Body.String())
		}
	}
	for _, unwanted := range []string{
		`data-routing-action="start"`,
	} {
		if strings.Contains(rec.Body.String(), unwanted) {
			t.Fatalf("dashboard should not expose %q:\n%s", unwanted, rec.Body.String())
		}
	}
}

func TestConfigPageIncludesRuntimeSettings(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{
		`id="settings-form"`,
		`name="autoStart"`,
		`name="pinDashboardClientRoute"`,
		`name="pinnedClientRoutes"`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("config page missing %q:\n%s", want, rec.Body.String())
		}
	}
}

func TestEventsEndpoint(t *testing.T) {
	srv := newTestServer(t, config.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Handler().ServeHTTP(rec, req)
	}()
	for i := 0; i < 50; i++ {
		if strings.Contains(rec.Body.String(), "event: status") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("events endpoint did not stop after client disconnect")
	}
	if got := rec.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "event: status") {
		t.Fatalf("unexpected body: %s", rec.Body.String())
	}
}

func TestSupervisorStatusRequiresAuth(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/supervisor/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSupervisorStatusReportsUnavailable(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/supervisor/status", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
