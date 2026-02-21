package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"time"
)

const (
	sessionCookieName = "eth_scanner_session"
	sessionDuration   = 24 * time.Hour
)

// handleLogin renders the login page or processes the login request.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		// If already authenticated, redirect to dashboard
		if s.isAuthenticated(r) {
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		s.renderer.Handler("login.html", map[string]any{"HideNav": true}).ServeHTTP(w, r)
		return
	}

	if r.Method == http.MethodPost {
		if err := r.ParseForm(); err != nil {
			http.Error(w, "failed to parse form", http.StatusBadRequest)
			return
		}

		password := r.FormValue("password")
		if s.cfg.DashboardPassword != "" && password == s.cfg.DashboardPassword {
			// Success - set cookie
			s.setSessionCookie(w)
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}

		// Failure - reload login with error
		s.renderer.Handler("login.html", map[string]any{
			"Error":   "Invalid password",
			"HideNav": true,
		}).ServeHTTP(w, r)
		return
	}

	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
}

// handleLogout clears the session cookie and redirects.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
	}
	http.SetCookie(w, cookie)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// isAuthenticated checks if the request has a valid session cookie.
func (s *Server) isAuthenticated(r *http.Request) bool {
	// If no password is set, dashboard is public.
	if s.cfg.DashboardPassword == "" {
		return true
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return false
	}

	// Simple check: compare cookie value to expected hash of password
	// We use a simple hash of the password itself to avoid storing it in plaintext
	// in the browser, though it's still static.
	expected := s.getSessionToken()
	return cookie.Value == expected
}

func (s *Server) setSessionCookie(w http.ResponseWriter) {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    s.getSessionToken(),
		Path:     "/",
		Expires:  time.Now().Add(sessionDuration),
		HttpOnly: true,
		// Secure should be true in production, but we don't know for sure here.
		// We'll leave it false for now to allow local testing over HTTP.
		SameSite: http.SameSiteLaxMode,
	}
	http.SetCookie(w, cookie)
}

func (s *Server) getSessionToken() string {
	// Simple static token based on the password
	h := sha256.New()
	h.Write([]byte(s.cfg.DashboardPassword))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// DashboardAuth is a middleware that protects dashboard routes.
func (s *Server) DashboardAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAuthenticated(r) {
			// Redirect to login if not authenticated
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}
