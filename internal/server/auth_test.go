package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"peplink-wg-bgp/internal/config"
)

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
