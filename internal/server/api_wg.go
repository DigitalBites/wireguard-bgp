package server

import (
	"io"
	"net/http"
	"os"
	"path/filepath"

	"peplink-wg-bgp/internal/supervisor"
	"peplink-wg-bgp/internal/wg"
)

func (s *Server) getWGConfig(w http.ResponseWriter, r *http.Request) {
	path := s.wgConfigPath()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		writeJSON(w, map[string]any{
			"path":   path,
			"exists": false,
			"meta":   wg.ConfigMeta{},
		})
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"path":   path,
		"exists": true,
		"meta":   wg.ParseConfig(string(data)),
	})
}

func (s *Server) getWGStatus(w http.ResponseWriter, r *http.Request) {
	s.supervisorAction(w, r, supervisor.ActionWGStatus)
}

func (s *Server) postWGStart(w http.ResponseWriter, r *http.Request) {
	s.supervisorAction(w, r, supervisor.ActionWGStart)
}

func (s *Server) postWGStop(w http.ResponseWriter, r *http.Request) {
	s.supervisorAction(w, r, supervisor.ActionWGStop)
}

func (s *Server) postWGRestart(w http.ResponseWriter, r *http.Request) {
	s.supervisorAction(w, r, supervisor.ActionWGRestart)
}

func (s *Server) postWGConfig(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_ = r.Body.Close()
	}()
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128*1024))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meta, err := wg.ValidateConfig(string(data))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	path := s.wgConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{
		"path":   path,
		"exists": true,
		"meta":   meta,
	})
}
