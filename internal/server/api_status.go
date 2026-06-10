package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"peplink-wg-bgp/internal/supervisor"
)

type runtimeStatus struct {
	Time      string        `json:"time"`
	WireGuard serviceStatus `json:"wireGuard"`
	BIRD      serviceStatus `json:"bird"`
}

type serviceStatus struct {
	State     string `json:"state"`
	Detail    string `json:"detail,omitempty"`
	Interface string `json:"interface,omitempty"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	wgInterface := s.cfg.WireGuard.Interface
	s.mu.RUnlock()
	writeJSON(w, runtimeStatus{
		Time:      time.Now().UTC().Format(time.RFC3339),
		WireGuard: s.wireGuardStatus(r, wgInterface),
		BIRD:      s.birdStatus(r),
	})
}

func (s *Server) wireGuardStatus(r *http.Request, iface string) serviceStatus {
	resp, err := s.supervisor.Call(r.Context(), supervisor.ActionWGStatus)
	status := serviceStatus{Interface: iface}
	if err != nil {
		status.State = "unavailable"
		status.Error = err.Error()
		return status
	}
	status.Detail = statusSummary(resp.Output)
	if wireGuardOutputHasInterface(resp.Output, iface) {
		status.State = "up"
		return status
	}
	status.State = "down"
	return status
}

func (s *Server) birdStatus(r *http.Request) serviceStatus {
	resp, err := s.supervisor.Call(r.Context(), supervisor.ActionBIRDStatus)
	if err != nil {
		return serviceStatus{State: "unavailable", Error: err.Error()}
	}
	return serviceStatus{
		State:  birdState(resp.Output),
		Detail: statusSummary(resp.Output),
	}
}

func wireGuardOutputHasInterface(output, iface string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "interface: "+iface {
			return true
		}
	}
	return false
}

func birdState(output string) string {
	lower := strings.ToLower(output)
	if strings.Contains(lower, "established") {
		return "up"
	}
	for _, line := range strings.Split(lower, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "peplink" && contains(fields, "up") {
			return "up"
		}
	}
	if strings.TrimSpace(output) == "" {
		return "down"
	}
	return "down"
}

func statusSummary(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			return line
		}
	}
	return ""
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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
