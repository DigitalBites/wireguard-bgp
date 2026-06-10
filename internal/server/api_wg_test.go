package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peplink-wg-bgp/internal/config"
	"peplink-wg-bgp/internal/supervisor"
)

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

func TestWGStatusCallsSupervisor(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "supervisor.sock")
	ctx, cancel := contextWithCancel()
	defer cancel()
	runner := &serverSupervisorTestRunner{output: "interface: wg0\n"}
	errCh := make(chan error, 1)
	go func() {
		errCh <- (supervisor.Server{
			SocketPath: socketPath,
			Runner:     runner,
		}).Serve(ctx)
	}()
	waitForSupervisor(t, socketPath)

	srv := newTestServer(t, config.Default())
	srv.supervisor = supervisor.Client{SocketPath: socketPath, Timeout: time.Second}
	req := httptest.NewRequest(http.MethodGet, "/api/wg/status", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if runner.commandLine() != "wg show" {
		t.Fatalf("unexpected command: %s", runner.commandLine())
	}
	if !strings.Contains(rec.Body.String(), "interface: wg0") {
		t.Fatalf("unexpected response: %s", rec.Body.String())
	}
	cancel()
	assertSupervisorStopped(t, errCh)
}

func TestWGStartRequiresCSRF(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodPost, "/api/wg/start", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestWGLifecycleCallsSupervisor(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		action   string
		expected string
	}{
		{name: "start", path: "/api/wg/start", action: "wg.start", expected: "started"},
		{name: "stop", path: "/api/wg/stop", action: "wg.stop", expected: "stopped"},
		{name: "restart", path: "/api/wg/restart", action: "wg.restart", expected: "restarted"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &serverWGManagerTestDouble{
				startOutput:   "started\n",
				stopOutput:    "stopped\n",
				restartOutput: "restarted\n",
			}
			runner, srv, stop := newSupervisorBackedTestServerWithWG(t, manager)
			defer stop()
			req := httptest.NewRequest(http.MethodPost, tt.path, nil)
			addCSRF(t, srv, req)
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
			}
			if runner.commandLine() != "" {
				t.Fatalf("wg lifecycle should use manager, got command %s", runner.commandLine())
			}
			if !strings.Contains(rec.Body.String(), `"action":"`+tt.action+`"`) || !strings.Contains(rec.Body.String(), tt.expected) {
				t.Fatalf("unexpected response: %s", rec.Body.String())
			}
		})
	}
}

type serverWGManagerTestDouble struct {
	startOutput   string
	stopOutput    string
	restartOutput string
}

func (m *serverWGManagerTestDouble) Start(ctx context.Context) (string, error) {
	return m.startOutput, nil
}

func (m *serverWGManagerTestDouble) Stop(ctx context.Context) (string, error) {
	return m.stopOutput, nil
}

func (m *serverWGManagerTestDouble) Restart(ctx context.Context) (string, error) {
	return m.restartOutput, nil
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
