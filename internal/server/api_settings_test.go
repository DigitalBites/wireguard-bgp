package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"peplink-wg-bgp/internal/config"
)

func TestSettingsDefaultsOff(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body settingsPayload
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.AutoStart || body.PinDashboardClientRoute || len(body.PinnedClientRoutes) != 0 {
		t.Fatalf("expected settings off by default: %#v", body)
	}
}

func TestSettingsSavePersistsRuntimeFlags(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ConfigDir = dir
	cfg.BIRDConfigPath = filepath.Join(dir, "bird", "bird.conf")
	srv := newTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"autoStart":true,"pinDashboardClientRoute":true,"pinnedClientRoutes":["192.168.64.1","192.168.64.1/32"]}`))
	req.Header.Set("Content-Type", "application/json")
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	got, err := config.Load(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !got.Runtime.AutoStart || !got.Runtime.PinDashboardClientRoute {
		t.Fatalf("runtime settings were not persisted: %#v", got.Runtime)
	}
	if strings.Join(got.Runtime.PinnedClientRoutes, ",") != "192.168.64.1/32" {
		t.Fatalf("pinned client routes were not normalized: %#v", got.Runtime.PinnedClientRoutes)
	}
}

func TestSettingsRejectsBroadPinnedClientRoute(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"pinnedClientRoutes":["192.168.64.0/24"]}`))
	req.Header.Set("Content-Type", "application/json")
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
