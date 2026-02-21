package server

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/garnizeh/eth-scanner/internal/config"
)

func TestHandleLogin_GET(t *testing.T) {
	cfg := &config.Config{DashboardPassword: "test-password"}
	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	t.Run("renders login page when not authenticated", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/login", nil)

		s.handleLogin(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "<form") {
			t.Errorf("expected login form in response, got: %s", rr.Body.String())
		}
	})

	t.Run("redirects to dashboard when already authenticated", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/login", nil)

		// Set valid session cookie
		token := s.getSessionToken()
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})

		s.handleLogin(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("expected status 303, got %d", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/dashboard" {
			t.Errorf("expected redirect to /dashboard, got %s", loc)
		}
	})
}

func TestHandleLogin_POST(t *testing.T) {
	password := "correct-password"
	cfg := &config.Config{DashboardPassword: password}
	s, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	t.Run("successful login sets cookie and redirects", func(t *testing.T) {
		form := url.Values{}
		form.Add("password", password)
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		s.handleLogin(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("expected status 303, got %d", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/dashboard" {
			t.Errorf("expected redirect to /dashboard, got %s", loc)
		}

		// Check cookie
		cookies := rr.Result().Cookies()
		var sessionCookie *http.Cookie
		for _, c := range cookies {
			if c.Name == sessionCookieName {
				sessionCookie = c
				break
			}
		}
		if sessionCookie == nil {
			t.Fatal("expected session cookie to be set")
		}

		expectedToken := s.getSessionToken()
		if sessionCookie.Value != expectedToken {
			t.Errorf("expected token %s, got %s", expectedToken, sessionCookie.Value)
		}
		if !sessionCookie.HttpOnly {
			t.Error("expected cookie to be HttpOnly")
		}
	})

	t.Run("failed login renders error message", func(t *testing.T) {
		form := url.Values{}
		form.Add("password", "wrong-password")
		req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		s.handleLogin(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if !strings.Contains(rr.Body.String(), "Invalid password") {
			t.Errorf("expected error message in body, got: %s", rr.Body.String())
		}

		// Ensure no valid cookie set
		cookies := rr.Result().Cookies()
		for _, c := range cookies {
			if c.Name == sessionCookieName && c.Value != "" && c.MaxAge >= 0 {
				t.Error("unexpected valid session cookie set on failed login")
			}
		}
	})
}

func TestHandleLogout(t *testing.T) {
	s, _ := New(&config.Config{DashboardPassword: "any"}, nil)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)

	s.handleLogout(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Errorf("expected status 303, got %d", rr.Code)
	}
	if loc := rr.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %s", loc)
	}

	// Check cookie is cleared
	cookies := rr.Result().Cookies()
	var cleared bool
	for _, c := range cookies {
		if c.Name == sessionCookieName && (c.Value == "" || c.MaxAge < 0) {
			cleared = true
			break
		}
	}
	if !cleared {
		t.Error("expected session cookie to be cleared")
	}
}

func TestDashboardAuthMiddleware(t *testing.T) {
	password := "secure-pass"
	cfg := &config.Config{DashboardPassword: password}
	s, _ := New(cfg, nil)

	handler := s.DashboardAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("protected content"))
	}))

	t.Run("redirects to login if no cookie", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("expected redirect status 303, got %d", rr.Code)
		}
		if loc := rr.Header().Get("Location"); loc != "/login" {
			t.Errorf("expected redirect to /login, got %s", loc)
		}
	})

	t.Run("redirects to login if invalid cookie", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: "invalid-token"})

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusSeeOther {
			t.Errorf("expected redirect status 303, got %d", rr.Code)
		}
	})

	t.Run("allows access with valid cookie", func(t *testing.T) {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)

		h := sha256.New()
		h.Write([]byte(password))
		token := fmt.Sprintf("%x", h.Sum(nil))
		req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})

		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rr.Code)
		}
		if rr.Body.String() != "protected content" {
			t.Errorf("unexpected body: %s", rr.Body.String())
		}
	})
}

func TestGetSessionToken(t *testing.T) {
	password := "test-secret"
	s := &Server{cfg: &config.Config{DashboardPassword: password}}

	token := s.getSessionToken()

	h := sha256.New()
	h.Write([]byte(password))
	expected := fmt.Sprintf("%x", h.Sum(nil))

	if token != expected {
		t.Errorf("expected token %s, got %s", expected, token)
	}
}

func TestDashboardAuth_NoPassword(t *testing.T) {
	// If DashboardPassword is empty, isAuthenticated should always return true.
	s, _ := New(&config.Config{DashboardPassword: ""}, nil)
	handler := s.DashboardAuth(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200 when no password is set, got %d", rr.Code)
	}
}

func TestHandleLogin_InvalidMethod(t *testing.T) {
	s, _ := New(&config.Config{DashboardPassword: "pass"}, nil)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/login", nil)

	s.handleLogin(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 Method Not Allowed, got %d", rr.Code)
	}
}

func TestHandleLogin_ParseFormError(t *testing.T) {
	s, _ := New(&config.Config{DashboardPassword: "pass"}, nil)
	rr := httptest.NewRecorder()
	// Malformed body with valid content type to trigger ParseForm error
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("!!invalid!!"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")

	// Trigger ParseForm error by providing an invalid escape sequence
	malformedBody := "password=%zz" // Invalid percent encoding
	req = httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(malformedBody))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	s.handleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request on malformed form, got %d", rr.Code)
	}
}
