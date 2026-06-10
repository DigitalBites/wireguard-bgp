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
	"peplink-wg-bgp/web"
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

func TestUnauthenticatedPageRedirectsToLogin(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Location"); got != "/login" {
		t.Fatalf("location = %q", got)
	}
}

func TestUnauthenticatedAPIReturnsUnauthorized(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLoginTokenCreatesSession(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/login?token=test-token", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Location"); got != "/" {
		t.Fatalf("location = %q", got)
	}
	if cookie := findCookie(rec.Result().Cookies(), "peplink_session"); cookie == nil || !cookie.HttpOnly {
		t.Fatalf("expected httponly session cookie, got %#v", rec.Result().Cookies())
	}
}

func TestAuthenticatedPageIncludesCSRFToken(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/config", nil)
	csrf := addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if csrf == "" || !strings.Contains(rec.Body.String(), `name="csrf-token" content="`+csrf+`"`) {
		t.Fatalf("page missing csrf token %q: %s", csrf, rec.Body.String())
	}
}

func TestPostRequiresCSRFToken(t *testing.T) {
	cfg := config.Default()
	cfg.ConfigDir = t.TempDir()
	cfg.BIRDConfigPath = filepath.Join(cfg.ConfigDir, "bird", "bird.conf")
	srv := newTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/wg/config", strings.NewReader(validWGConfig()))
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPostRejectsInvalidCSRFToken(t *testing.T) {
	cfg := config.Default()
	cfg.ConfigDir = t.TempDir()
	cfg.BIRDConfigPath = filepath.Join(cfg.ConfigDir, "bird", "bird.conf")
	srv := newTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/wg/config", strings.NewReader(validWGConfig()))
	addTestSession(t, srv, req)
	req.Header.Set("X-CSRF-Token", "wrong-token")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
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

func TestWireGuardConfigPostAndGet(t *testing.T) {
	cfg := config.Default()
	cfg.ConfigDir = t.TempDir()
	cfg.BIRDConfigPath = filepath.Join(cfg.ConfigDir, "bird", "bird.conf")
	srv := newTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/wg/config", strings.NewReader(validWGConfig()))
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("post status = %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "private-value") {
		t.Fatalf("response leaked private key: %s", rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/api/wg/config", nil)
	addTestSession(t, srv, req)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"exists":true`) {
		t.Fatalf("unexpected get response: %s", rec.Body.String())
	}
}

func TestWireGuardConfigPostRejectsInvalidConfig(t *testing.T) {
	cfg := config.Default()
	cfg.ConfigDir = t.TempDir()
	cfg.BIRDConfigPath = filepath.Join(cfg.ConfigDir, "bird", "bird.conf")
	srv := newTestServer(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/api/wg/config", strings.NewReader("not a wg config"))
	addCSRF(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

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

func TestServerRejectsManagedPathOutsideStateDir(t *testing.T) {
	cfg := config.Default()
	cfg.BIRDConfigPath = "/etc/bird.conf"
	if _, err := NewWithAuth(cfg, web.Templates, web.Static, AuthConfig{
		Token:      "test-token",
		SessionTTL: time.Hour,
	}); err == nil {
		t.Fatal("expected managed path validation error")
	}
}

func newTestServer(t *testing.T, cfg config.App) *Server {
	t.Helper()
	srv, err := NewWithAuth(cfg, web.Templates, web.Static, AuthConfig{
		Token:      "test-token",
		SessionTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func addTestSession(t *testing.T, srv *Server, req *http.Request) string {
	t.Helper()
	rec := httptest.NewRecorder()
	loginReq := httptest.NewRequest(http.MethodGet, "/login?token=test-token", nil)
	srv.Handler().ServeHTTP(rec, loginReq)
	cookie := findCookie(rec.Result().Cookies(), "peplink_session")
	if cookie == nil {
		t.Fatalf("login did not set session cookie: %#v", rec.Result().Cookies())
	}
	req.AddCookie(cookie)
	session, ok := srv.auth.sessionForRequest(req)
	if !ok {
		t.Fatal("login session was not accepted")
	}
	return session.CSRFToken
}

func addCSRF(t *testing.T, srv *Server, req *http.Request) {
	t.Helper()
	csrf := addTestSession(t, srv, req)
	req.Header.Set("X-CSRF-Token", csrf)
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func validWGConfig() string {
	return `[Interface]
PrivateKey = private-value

[Peer]
PublicKey = abcdefghijklmnopqrstuvwxyz1234567890
Endpoint = 172.17.62.1:51820
AllowedIPs = 0.0.0.0/0
`
}
