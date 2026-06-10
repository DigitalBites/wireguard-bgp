package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"peplink-wg-bgp/internal/supervisor"
)

const statusEventInterval = 10 * time.Second

type runtimeStatus struct {
	Time      string        `json:"time"`
	WireGuard serviceStatus `json:"wireGuard"`
	BIRD      serviceStatus `json:"bird"`
}

type serviceStatus struct {
	State     string            `json:"state"`
	Detail    string            `json:"detail,omitempty"`
	Interface string            `json:"interface,omitempty"`
	Error     string            `json:"error,omitempty"`
	Stats     map[string]string `json:"stats,omitempty"`
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
		status.Stats = s.wireGuardStats(r)
		return status
	}
	status.State = "down"
	return status
}

func (s *Server) wireGuardStats(r *http.Request) map[string]string {
	resp, err := s.supervisor.Call(r.Context(), supervisor.ActionWGDump)
	if err != nil || !resp.OK {
		return nil
	}
	return parseWireGuardDump(resp.Output)
}

func (s *Server) birdStatus(r *http.Request) serviceStatus {
	resp, err := s.supervisor.Call(r.Context(), supervisor.ActionBIRDStatus)
	if err != nil {
		return serviceStatus{State: "unavailable", Error: err.Error()}
	}
	return serviceStatus{
		State:  birdState(resp.Output),
		Detail: statusSummary(resp.Output),
		Stats:  s.birdStats(r),
	}
}

func (s *Server) birdStats(r *http.Request) map[string]string {
	resp, err := s.supervisor.Call(r.Context(), supervisor.ActionBIRDDetails)
	if err != nil || !resp.OK {
		return nil
	}
	return parseBIRDDetails(resp.Output)
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

func parseWireGuardDump(output string) map[string]string {
	stats := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) == 4 {
			stats["listenPort"] = emptyAsUnknown(fields[2])
			stats["fwmark"] = emptyAsUnknown(fields[3])
			continue
		}
		if len(fields) >= 8 {
			stats["peer"] = redactPeerKey(fields[0])
			stats["endpoint"] = emptyAsUnknown(fields[2])
			stats["allowedIPs"] = emptyAsUnknown(fields[3])
			stats["latestHandshake"] = formatHandshake(fields[4])
			stats["rxBytes"] = fields[5]
			stats["txBytes"] = fields[6]
			stats["persistentKeepalive"] = emptyAsUnknown(fields[7])
			break
		}
	}
	if len(stats) == 0 {
		return nil
	}
	return stats
}

func parseBIRDDetails(output string) map[string]string {
	stats := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(line, "Description:"):
			stats["description"] = strings.TrimSpace(strings.TrimPrefix(line, "Description:"))
		case strings.HasPrefix(line, "Neighbor address:"):
			stats["neighbor"] = strings.TrimSpace(strings.TrimPrefix(line, "Neighbor address:"))
		case strings.HasPrefix(line, "Neighbor AS:"):
			stats["neighborAS"] = strings.TrimSpace(strings.TrimPrefix(line, "Neighbor AS:"))
		case strings.HasPrefix(line, "Local AS:"):
			stats["localAS"] = strings.TrimSpace(strings.TrimPrefix(line, "Local AS:"))
		case strings.HasPrefix(line, "Routes:"):
			stats["routes"] = strings.TrimSpace(strings.TrimPrefix(line, "Routes:"))
		case strings.Contains(lower, "bgp state:"):
			_, value, _ := strings.Cut(line, ":")
			stats["bgpState"] = strings.TrimSpace(value)
		}
	}
	if len(stats) == 0 {
		return nil
	}
	return stats
}

func formatHandshake(raw string) string {
	seconds, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || seconds <= 0 {
		return "never"
	}
	t := time.Unix(seconds, 0).UTC()
	age := time.Since(t)
	if age < 0 {
		age = 0
	}
	return fmt.Sprintf("%s ago", age.Round(time.Second))
}

func emptyAsUnknown(value string) string {
	if value == "" || value == "(none)" || value == "0" {
		return "none"
	}
	return value
}

func redactPeerKey(value string) string {
	if len(value) <= 12 {
		return value
	}
	return value[:6] + "..." + value[len(value)-6:]
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	if err := writeStatusEvent(w); err != nil {
		return
	}
	flusher.Flush()
	ticker := time.NewTicker(statusEventInterval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if err := writeStatusEvent(w); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func writeStatusEvent(w http.ResponseWriter) error {
	payload, _ := json.Marshal(map[string]any{
		"time": time.Now().UTC().Format(time.RFC3339),
		"type": "status",
	})
	if _, err := fmt.Fprintf(w, "event: status\ndata: %s\n\n", payload); err != nil {
		return err
	}
	return nil
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
