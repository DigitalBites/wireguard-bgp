package server

import (
	"embed"
	"html/template"
	"net/http"
	"sync"
	"time"

	"peplink-wg-bgp/internal/config"
	"peplink-wg-bgp/internal/diag"
	"peplink-wg-bgp/internal/supervisor"
)

type Server struct {
	cfg        config.App
	mu         sync.RWMutex
	templates  *template.Template
	static     embed.FS
	diag       diag.Runner
	supervisor supervisor.Client
	auth       *Auth
}

func New(cfg config.App, templates embed.FS, static embed.FS) (*Server, error) {
	token, err := GenerateToken()
	if err != nil {
		return nil, err
	}
	return NewWithAuth(cfg, templates, static, AuthConfig{Token: token, SessionTTL: time.Hour})
}

func NewWithAuth(cfg config.App, templates embed.FS, static embed.FS, authConfig AuthConfig) (*Server, error) {
	if err := config.ValidateManagedPaths(cfg); err != nil {
		return nil, err
	}
	tpl, err := template.ParseFS(templates, "templates/*.html")
	if err != nil {
		return nil, err
	}
	auth, err := NewAuth(authConfig)
	if err != nil {
		return nil, err
	}
	return &Server{
		cfg:        cfg,
		templates:  tpl,
		static:     static,
		diag:       diag.Runner{},
		supervisor: supervisor.Client{},
		auth:       auth,
	}, nil
}

func (s *Server) LoginURL() string {
	return "/login?token=" + s.auth.loginToken
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", s.loginPage)
	mux.HandleFunc("POST /login", s.loginPost)
	mux.HandleFunc("GET /", s.page("dashboard.html"))
	mux.HandleFunc("GET /config", s.page("config.html"))
	mux.HandleFunc("GET /diagnostics", s.page("diagnostics.html"))
	mux.HandleFunc("GET /logs", s.page("logs.html"))
	mux.Handle("GET /static/", http.FileServerFS(s.static))
	mux.HandleFunc("GET /api/status", s.status)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("GET /api/supervisor/status", s.supervisorStatus)
	mux.HandleFunc("GET /api/wg/config", s.getWGConfig)
	mux.HandleFunc("POST /api/wg/config", s.postWGConfig)
	mux.HandleFunc("GET /api/wg/status", s.getWGStatus)
	mux.HandleFunc("GET /api/bird/config", s.getBIRDConfig)
	mux.HandleFunc("POST /api/bird/config", s.postBIRDConfig)
	mux.HandleFunc("POST /api/bird/start", s.postBIRDStart)
	mux.HandleFunc("POST /api/bird/reload", s.postBIRDReload)
	mux.HandleFunc("GET /api/bird/status", s.getBIRDStatus)
	for _, name := range []string{"routes", "addresses", "rules", "links", "neighbors", "wg", "bird", "bird-routes"} {
		mux.HandleFunc("GET /api/diag/"+name, s.diagHandler(name))
	}
	return s.requireAuth(mux)
}
