package server

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"Title":        pageTitle(name),
			"Active":       strings.TrimSuffix(name, ".html"),
			"Generated":    time.Now().UTC().Format(time.RFC3339),
			"CSRFToken":    s.auth.csrfToken(r),
			"BuildVersion": s.buildVersion,
		}
		if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

func pageTitle(name string) string {
	switch name {
	case "dashboard.html":
		return "Dashboard"
	case "config.html":
		return "Configuration"
	case "diagnostics.html":
		return "Diagnostics"
	case "logs.html":
		return "Logs"
	default:
		return "Peplink WG BGP"
	}
}
