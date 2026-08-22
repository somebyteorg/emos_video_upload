package api

import (
	"net/http"
	"strings"
)

type Auth struct {
	username string
	password string
}

func NewAuth(username, password string) *Auth {
	return &Auth{username: username, password: password}
}

func (a *Auth) Enabled() bool {
	return strings.TrimSpace(a.username) != "" || strings.TrimSpace(a.password) != ""
}

func (a *Auth) ValidCredentials(username, password string) bool {
	if !a.Enabled() {
		return true
	}
	return username == a.username && password == a.password
}

func (a *Auth) ValidRequest(r *http.Request) bool {
	if !a.Enabled() {
		return true
	}
	username, password, ok := r.BasicAuth()
	return ok && a.ValidCredentials(username, password)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if !s.auth.ValidRequest(r) {
		s.requireAuth(w)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": true})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	authenticated := s.auth.ValidRequest(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"authenticated": authenticated,
		"enabled":       s.auth.Enabled(),
		"file_storages": s.cfg.EMOSFileStorage,
	})
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.auth.ValidRequest(r) {
			s.requireAuth(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="EMOS Upload"`)
	writeError(w, http.StatusUnauthorized, "valid Authorization Basic credentials are required")
}
