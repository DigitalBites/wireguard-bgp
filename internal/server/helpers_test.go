package server

import (
	"context"
	"embed"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peplink-wg-bgp/internal/config"
	"peplink-wg-bgp/internal/supervisor"
	"peplink-wg-bgp/web"
)

func newTestServer(t *testing.T, cfg config.App) *Server {
	t.Helper()
	srv, err := NewWithAuth(cfg, webTemplates(), webStatic(), AuthConfig{
		Token:      "test-token",
		SessionTTL: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	return srv
}

func webTemplates() embed.FS {
	return web.Templates
}

func webStatic() embed.FS {
	return web.Static
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

func newSupervisorBackedTestServer(t *testing.T) (*serverSupervisorTestRunner, *Server, func()) {
	t.Helper()
	return newSupervisorBackedTestServerWithWG(t, nil)
}

func newSupervisorBackedTestServerWithWG(t *testing.T, manager supervisor.WGManager) (*serverSupervisorTestRunner, *Server, func()) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "supervisor.sock")
	ctx, cancel := context.WithCancel(context.Background())
	runner := &serverSupervisorTestRunner{}
	errCh := make(chan error, 1)
	go func() {
		errCh <- (supervisor.Server{
			SocketPath: socketPath,
			Runner:     runner,
			WGManager:  manager,
		}).Serve(ctx)
	}()
	waitForSupervisor(t, socketPath)

	srv := newTestServer(t, config.Default())
	srv.supervisor = supervisor.Client{SocketPath: socketPath, Timeout: time.Second}
	return runner, srv, func() {
		cancel()
		assertSupervisorStopped(t, errCh)
	}
}

func contextWithCancel() (context.Context, context.CancelFunc) {
	return context.WithCancel(context.Background())
}

func waitForSupervisor(t *testing.T, socketPath string) {
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

func assertSupervisorStopped(t *testing.T, errCh <-chan error) {
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

type serverSupervisorTestRunner struct {
	name   string
	args   []string
	calls  []string
	output string
}

func (r *serverSupervisorTestRunner) Run(ctx context.Context, name string, args ...string) (string, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	r.calls = append(r.calls, strings.TrimSpace(name+" "+strings.Join(args, " ")))
	if r.output != "" {
		return r.output, nil
	}
	return "configured\n", nil
}

func (r *serverSupervisorTestRunner) commandLine() string {
	return strings.TrimSpace(r.name + " " + strings.Join(r.args, " "))
}

func (r *serverSupervisorTestRunner) commandLines() []string {
	return append([]string(nil), r.calls...)
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
