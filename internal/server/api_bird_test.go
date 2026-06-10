package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peplink-wg-bgp/internal/config"
)

func TestBirdConfigPostSavesAppConfigAndGeneratedConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.ConfigDir = dir
	cfg.BIRDConfigPath = filepath.Join(dir, "bird.conf")
	srv := newTestServer(t, cfg)
	body := `{
		"routerId":"192.168.50.67",
		"localAsn":65060,
		"peerAsn":65001,
		"peerIp":"192.168.50.1",
		"advertisedRoutes":["0.0.0.0/1","128.0.0.0/1"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/bird/config", strings.NewReader(body))
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" {
		t.Fatalf("unexpected response: %#v", resp)
	}
	birdConf, err := os.ReadFile(filepath.Join(dir, "bird.conf"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(birdConf), "neighbor 192.168.50.1 as 65001;") {
		t.Fatalf("unexpected bird config:\n%s", string(birdConf))
	}
	appConfig, err := os.ReadFile(filepath.Join(dir, "app.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(appConfig), "localAsn: 65060") {
		t.Fatalf("app config did not persist BIRD settings:\n%s", string(appConfig))
	}
}

func TestBirdConfigGetReturnsGeneratedPreview(t *testing.T) {
	cfg := config.Default()
	cfg.ConfigDir = t.TempDir()
	cfg.BIRDConfigPath = filepath.Join(cfg.ConfigDir, "bird", "bird.conf")
	cfg.BIRD.LocalASN = 65060
	cfg.BIRD.PeerASN = 65001
	cfg.BIRD.PeerIP = "192.168.50.1"
	srv := newTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodGet, "/api/bird/config", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"generated"`) || !strings.Contains(rec.Body.String(), `neighbor 192.168.50.1 as 65001`) {
		t.Fatalf("unexpected preview response: %s", rec.Body.String())
	}
}

func TestBirdConfigPostRejectsInvalidRoute(t *testing.T) {
	cfg := config.Default()
	cfg.ConfigDir = t.TempDir()
	cfg.BIRDConfigPath = filepath.Join(cfg.ConfigDir, "bird.conf")
	srv := newTestServer(t, cfg)
	body := `{
		"localAsn":65060,
		"peerAsn":65001,
		"peerIp":"192.168.50.1",
		"advertisedRoutes":["not-a-route"]
	}`
	req := httptest.NewRequest(http.MethodPost, "/api/bird/config", strings.NewReader(body))
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(cfg.BIRDConfigPath); !os.IsNotExist(err) {
		t.Fatalf("invalid config should not write bird config, stat err=%v", err)
	}
}

func TestBirdReloadRequiresCSRF(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/bird/reload", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestBirdReloadCallsSupervisor(t *testing.T) {
	runner, srv, stop := newSupervisorBackedTestServer(t)
	defer stop()
	req := httptest.NewRequest(http.MethodPost, "/api/bird/reload", nil)
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"action":"bird.reload"`) || !strings.Contains(rec.Body.String(), "configured") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	if runner.commandLine() != "birdc -s /run/bird/bird.ctl configure" {
		t.Fatalf("unexpected command: %s", runner.commandLine())
	}
}

func TestBirdStartCallsSupervisor(t *testing.T) {
	runner, srv, stop := newSupervisorBackedTestServer(t)
	defer stop()
	req := httptest.NewRequest(http.MethodPost, "/api/bird/start", nil)
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if runner.commandLine() != "birdc -s /run/bird/bird.ctl show status" {
		t.Fatalf("unexpected command: %s", runner.commandLine())
	}
	if !strings.Contains(rec.Body.String(), "bird already running") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
}

func TestBirdStatusCallsSupervisor(t *testing.T) {
	runner, srv, stop := newSupervisorBackedTestServer(t)
	defer stop()
	srv.supervisor.Timeout = time.Second
	req := httptest.NewRequest(http.MethodGet, "/api/bird/status", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if runner.commandLine() != "birdc -s /run/bird/bird.ctl show protocols" {
		t.Fatalf("unexpected command: %s", runner.commandLine())
	}
}

func TestServerRejectsManagedPathOutsideStateDir(t *testing.T) {
	cfg := config.Default()
	cfg.BIRDConfigPath = "/etc/bird.conf"
	if _, err := NewWithAuth(cfg, webTemplates(), webStatic(), AuthConfig{
		Token:      "test-token",
		SessionTTL: time.Hour,
	}); err == nil {
		t.Fatal("expected managed path validation error")
	}
}
