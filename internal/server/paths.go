package server

import (
	"os"
	"path/filepath"
)

func (s *Server) wgConfigPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return filepath.Join(s.cfg.ConfigDir, "wireguard", "wg0.conf")
}

func (s *Server) appConfigPath() string {
	return filepath.Join(s.cfg.ConfigDir, "app.yaml")
}

func writeFileCreatingDir(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}
