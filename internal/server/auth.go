package server

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type AuthConfig struct {
	Token        string
	SessionTTL   time.Duration
	CookieSecure bool
	BuildVersion string
}

type Auth struct {
	loginToken   string
	sessionTTL   time.Duration
	cookieSecure bool
	mu           sync.Mutex
	sessions     map[string]Session
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
		loginToken:   token,
		sessionTTL:   ttl,
		cookieSecure: cfg.CookieSecure,
		sessions:     make(map[string]Session),
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
		Secure:   a.cookieSecure,
		SameSite: http.SameSiteStrictMode,
	})
	return nil
}
