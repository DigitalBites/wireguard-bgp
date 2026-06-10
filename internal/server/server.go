package server

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"peplink-wg-bgp/internal/bird"
	"peplink-wg-bgp/internal/config"
	"peplink-wg-bgp/internal/diag"
	"peplink-wg-bgp/internal/supervisor"
	"peplink-wg-bgp/internal/wg"
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
	mux.HandleFunc("GET /api/bird/config", s.getBIRDConfig)
	mux.HandleFunc("POST /api/bird/config", s.postBIRDConfig)
	for _, name := range []string{"routes", "addresses", "rules", "links", "neighbors", "wg", "bird", "bird-routes"} {
		mux.HandleFunc("GET /api/diag/"+name, s.diagHandler(name))
	}
	return s.requireAuth(mux)
}

type AuthConfig struct {
	Token      string
	SessionTTL time.Duration
}

type Auth struct {
	loginToken string
	sessionTTL time.Duration
	mu         sync.Mutex
	sessions   map[string]Session
}

type Session struct {
	Expires   time.Time
	CSRFToken string
}

func NewAuth(cfg AuthConfig) (*Auth, error) {
	token := cfg.Token
	var err error
	if token == "" {
		token, err = GenerateToken()
		if err != nil {
			return nil, err
		}
	}
	ttl := cfg.SessionTTL
	if ttl == 0 {
		ttl = time.Hour
	}
	return &Auth{
		loginToken: token,
		sessionTTL: ttl,
		sessions:   make(map[string]Session),
	}, nil
}

func GenerateToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || strings.HasPrefix(r.URL.Path, "/static/") {
			next.ServeHTTP(w, r)
			return
		}
		if session, ok := s.auth.sessionForRequest(r); ok {
			if isUnsafeMethod(r.Method) && !s.auth.validCSRF(r, session) {
				if strings.HasPrefix(r.URL.Path, "/api/") {
					writeJSONStatus(w, http.StatusForbidden, map[string]any{"error": "invalid csrf token"})
					return
				}
				http.Error(w, "invalid csrf token", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONStatus(w, http.StatusUnauthorized, map[string]any{"error": "authentication required"})
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	})
}

func (s *Server) loginPage(w http.ResponseWriter, r *http.Request) {
	if token := r.URL.Query().Get("token"); token != "" {
		if s.auth.matchesToken(token) {
			if err := s.auth.setSession(w); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
	}
	data := map[string]any{
		"Title": "Login",
	}
	if err := s.templates.ExecuteTemplate(w, "login.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) loginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid login form", http.StatusBadRequest)
		return
	}
	if !s.auth.matchesToken(r.FormValue("token")) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	if err := s.auth.setSession(w); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Auth) matchesToken(token string) bool {
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(a.loginToken)) == 1
}

func (a *Auth) sessionForRequest(r *http.Request) (Session, bool) {
	cookie, err := r.Cookie("peplink_session")
	if err != nil || cookie.Value == "" {
		return Session{}, false
	}
	now := time.Now()
	a.mu.Lock()
	defer a.mu.Unlock()
	session, ok := a.sessions[cookie.Value]
	if !ok {
		return Session{}, false
	}
	if now.After(session.Expires) {
		delete(a.sessions, cookie.Value)
		return Session{}, false
	}
	return session, true
}

func (a *Auth) csrfToken(r *http.Request) string {
	session, ok := a.sessionForRequest(r)
	if !ok {
		return ""
	}
	return session.CSRFToken
}

func (a *Auth) validCSRF(r *http.Request, session Session) bool {
	token := r.Header.Get("X-CSRF-Token")
	if token == "" || session.CSRFToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(session.CSRFToken)) == 1
}

func (a *Auth) setSession(w http.ResponseWriter) error {
	sessionID, err := GenerateToken()
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}
	csrfToken, err := GenerateToken()
	if err != nil {
		return fmt.Errorf("failed to create csrf token: %w", err)
	}
	expires := time.Now().Add(a.sessionTTL)
	a.mu.Lock()
	a.sessions[sessionID] = Session{
		Expires:   expires,
		CSRFToken: csrfToken,
	}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{
		Name:     "peplink_session",
		Value:    sessionID,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(a.sessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}

func (s *Server) page(name string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data := map[string]any{
			"Title":     pageTitle(name),
			"Active":    strings.TrimSuffix(name, ".html"),
			"Generated": time.Now().UTC().Format(time.RFC3339),
			"CSRFToken": s.auth.csrfToken(r),
		}
		if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	}
}

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
	fmt.Fprintf(w, "event: status\ndata: %s\n\n", payload)
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

func (s *Server) postWGConfig(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 128*1024))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	meta := wg.ParseConfig(string(data))
	if !meta.HasPrivateKey || meta.PeerPublicKey == "" {
		http.Error(w, "wireguard config must include interface private key and peer public key", http.StatusBadRequest)
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
	defer r.Body.Close()
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

func writeJSON(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONStatus(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func isUnsafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return false
	default:
		return true
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
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
