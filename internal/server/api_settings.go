package server

import (
	"encoding/json"
	"net/http"

	"peplink-wg-bgp/internal/config"
)

type settingsPayload struct {
	AutoStart               bool     `json:"autoStart"`
	PinDashboardClientRoute bool     `json:"pinDashboardClientRoute"`
	PinnedClientRoutes      []string `json:"pinnedClientRoutes"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	runtime := s.cfg.Runtime
	s.mu.RUnlock()
	writeJSON(w, settingsPayload{
		AutoStart:               runtime.AutoStart,
		PinDashboardClientRoute: runtime.PinDashboardClientRoute,
		PinnedClientRoutes:      runtime.PinnedClientRoutes,
	})
}

func (s *Server) postSettings(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_ = r.Body.Close()
	}()
	var next settingsPayload
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024)).Decode(&next); err != nil {
		http.Error(w, "invalid settings JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	cfg := s.cfg
	appConfigPath := s.appConfigPath()
	s.mu.RUnlock()
	pinnedRoutes, err := config.NormalizePinnedClientRoutes(next.PinnedClientRoutes)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	cfg.Runtime = config.Runtime{
		AutoStart:               next.AutoStart,
		PinDashboardClientRoute: next.PinDashboardClientRoute,
		PinnedClientRoutes:      pinnedRoutes,
	}
	if err := config.Save(appConfigPath, cfg); err != nil {
		http.Error(w, "save app config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	s.logs.Add("info", "runtime settings saved", appConfigPath)
	next.PinnedClientRoutes = pinnedRoutes
	writeJSON(w, next)
}
