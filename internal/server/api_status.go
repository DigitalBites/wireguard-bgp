package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"peplink-wg-bgp/internal/supervisor"
)

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	wgInterface := s.cfg.WireGuard.Interface
	s.mu.RUnlock()
	writeJSON(w, map[string]any{
		"time":      time.Now().UTC().Format(time.RFC3339),
		"wireGuard": map[string]any{"interface": wgInterface, "state": "unknown"},
		"bird":      map[string]any{"state": "unknown"},
	})
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	payload, _ := json.Marshal(map[string]any{
		"time": time.Now().UTC().Format(time.RFC3339),
		"type": "status",
	})
	if _, err := fmt.Fprintf(w, "event: status\ndata: %s\n\n", payload); err != nil {
		return
	}
}

func (s *Server) supervisorStatus(w http.ResponseWriter, r *http.Request) {
	resp, err := s.supervisor.Call(r.Context(), supervisor.ActionPing)
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
			"ok":    false,
			"error": err.Error(),
		})
		return
	}
	writeJSON(w, resp)
}
