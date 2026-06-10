package server

import "net/http"

func (s *Server) diagHandler(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := s.diag.Run(r.Context(), name)
		status := "ok"
		if err != nil {
			status = "error"
		}
		writeJSON(w, map[string]any{
			"name":   name,
			"status": status,
			"output": out,
			"error":  errorString(err),
		})
	}
}
