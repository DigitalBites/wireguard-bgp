package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

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
		"wg show": "interface: wg0\n  public key: abc\n",
		"birdc -s /run/bird/bird.ctl show protocols": "peplink BGP master up 2026-06-10 Established\n",
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
	wantCommands := []string{
		"wg show",
		"birdc -s /run/bird/bird.ctl show protocols",
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
		`data-routing-action="start"`,
		`data-routing-action="apply"`,
		`data-routing-action="restart"`,
		`data-routing-action="stop"`,
		`id="routing-steps"`,
	} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("dashboard missing %q:\n%s", want, rec.Body.String())
		}
	}
}

func TestEventsEndpoint(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
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
