package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peplink-wg-bgp/internal/config"
)

func TestLogsEndpointReturnsEntries(t *testing.T) {
	srv := newTestServer(t, config.Default())
	srv.logs.Add("info", "test event", "detail")
	req := httptest.NewRequest(http.MethodGet, "/api/logs", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Entries []LogEntry `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Entries) < 2 || body.Entries[len(body.Entries)-1].Message != "test event" {
		t.Fatalf("unexpected entries: %#v", body.Entries)
	}
}

func TestLogsPageIncludesLogViewer(t *testing.T) {
	srv := newTestServer(t, config.Default())
	req := httptest.NewRequest(http.MethodGet, "/logs", nil)
	addTestSession(t, srv, req)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	for _, want := range []string{`id="app-logs"`, `id="refresh-logs"`, `/static/app.js`} {
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("logs page missing %q:\n%s", want, rec.Body.String())
		}
	}
}
