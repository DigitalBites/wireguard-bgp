package server

import (
	"encoding/json"
	"net/http"

	"peplink-wg-bgp/internal/bird"
	"peplink-wg-bgp/internal/config"
	"peplink-wg-bgp/internal/supervisor"
)

func (s *Server) getBIRDConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.cfg.BIRD
	path := s.cfg.BIRDConfigPath
	s.mu.RUnlock()
	generated, err := bird.Generate(cfg)
	status := "ok"
	if err != nil {
		status = "invalid"
	}
	writeJSON(w, map[string]any{
		"path":      path,
		"config":    cfg,
		"generated": generated,
		"status":    status,
		"error":     errorString(err),
	})
}

func (s *Server) postBIRDConfig(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_ = r.Body.Close()
	}()
	var next bird.Config
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64*1024)).Decode(&next); err != nil {
		http.Error(w, "invalid BIRD config JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	next = next.WithDefaults()
	generated, err := bird.Generate(next)
	if err != nil {
		writeJSONStatus(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	s.mu.RLock()
	cfg := s.cfg
	appConfigPath := s.appConfigPath()
	birdConfigPath := s.cfg.BIRDConfigPath
	s.mu.RUnlock()
	cfg.BIRD = next

	if err := writeFileCreatingDir(birdConfigPath, []byte(generated), 0o600); err != nil {
		http.Error(w, "write BIRD config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if err := config.Save(appConfigPath, cfg); err != nil {
		http.Error(w, "save app config: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	s.cfg = cfg
	s.mu.Unlock()
	writeJSON(w, map[string]any{
		"path":      birdConfigPath,
		"appConfig": appConfigPath,
		"config":    next,
		"generated": generated,
		"status":    "ok",
	})
}

func (s *Server) postBIRDReload(w http.ResponseWriter, r *http.Request) {
	s.supervisorAction(w, r, supervisor.ActionBIRDReload)
}

func (s *Server) postBIRDStart(w http.ResponseWriter, r *http.Request) {
	s.supervisorAction(w, r, supervisor.ActionBIRDStart)
}

func (s *Server) getBIRDStatus(w http.ResponseWriter, r *http.Request) {
	s.supervisorAction(w, r, supervisor.ActionBIRDStatus)
}

func (s *Server) supervisorAction(w http.ResponseWriter, r *http.Request, action string) {
	resp, err := s.supervisor.Call(r.Context(), action)
	if err != nil {
		writeJSONStatus(w, http.StatusServiceUnavailable, map[string]any{
			"ok":     false,
			"action": action,
			"output": resp.Output,
			"error":  err.Error(),
		})
		return
	}
	writeJSON(w, resp)
}
