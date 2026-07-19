package mux

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

func TestAdminRedirects(t *testing.T) {
	app := web.NewApp(func(context.Context, string, ...any) {})
	registerAdminRoutes(app, Config{})
	app.NotFoundHandler()

	for _, path := range []string{"/", "/admin"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://example.test"+path, nil)
			rr := httptest.NewRecorder()

			app.ServeHTTP(rr, req)

			if rr.Code != http.StatusPermanentRedirect {
				t.Errorf("status: got %d, want %d", rr.Code, http.StatusPermanentRedirect)
			}
			if location := rr.Header().Get("Location"); location != "/admin/" {
				t.Errorf("Location: got %q, want %q", location, "/admin/")
			}
		})
	}
}

func TestAdminCookieMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer cookie-token" {
			t.Errorf("Authorization: got %q, want %q", got, "Bearer cookie-token")
		}
		w.WriteHeader(http.StatusNoContent)
	})

	t.Run("promotes cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "https://example.test/v1/models", nil)
		req.AddCookie(&http.Cookie{Name: adminHTTPSCookieName, Value: "cookie-token"})
		rr := httptest.NewRecorder()
		adminCookieMiddleware(next).ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Errorf("status: got %d, want %d", rr.Code, http.StatusNoContent)
		}
	})

	t.Run("rejects unsafe cross origin cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "https://example.test/v1/security/keys/add", nil)
		req.Header.Set("Origin", "https://evil.test")
		req.AddCookie(&http.Cookie{Name: adminHTTPSCookieName, Value: "cookie-token"})
		rr := httptest.NewRecorder()
		adminCookieMiddleware(next).ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("status: got %d, want %d", rr.Code, http.StatusForbidden)
		}
	})
}

func TestSetAdminCookie(t *testing.T) {
	tests := []struct {
		name   string
		url    string
		proto  string
		cookie string
		secure bool
	}{
		{"https", "https://example.test/admin/api/login", "", adminHTTPSCookieName, true},
		{"https at ingress", "http://example.test/admin/api/login", "https", adminHTTPSCookieName, true},
		{"http", "http://example.test/admin/api/login", "", adminHTTPCookieName, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tt.url, nil)
			if tt.proto != "" {
				req.Header.Set("X-Forwarded-Proto", tt.proto)
			}
			rr := httptest.NewRecorder()
			setAdminCookie(rr, req, "token", time.Now().Add(time.Hour), 3600)
			cookies := rr.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatalf("cookies: got %d, want 1", len(cookies))
			}
			cookie := cookies[0]
			if cookie.Name != tt.cookie || !cookie.HttpOnly || cookie.Secure != tt.secure || cookie.Path != "/" || cookie.SameSite != http.SameSiteStrictMode {
				t.Errorf("cookie attributes: got %+v", cookie)
			}
		})
	}
}

func TestClearAdminCookiesHTTPS(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "https://example.test/admin/api/logout", nil)
	rr := httptest.NewRecorder()

	clearAdminCookies(rr, req)

	cookies := rr.Result().Cookies()
	if len(cookies) != 2 {
		t.Fatalf("cookies: got %d, want 2", len(cookies))
	}
	if cookies[0].Name != adminHTTPSCookieName || cookies[1].Name != adminHTTPCookieName {
		t.Errorf("cookie names: got %q and %q", cookies[0].Name, cookies[1].Name)
	}
	for _, cookie := range cookies {
		if cookie.MaxAge != -1 || !cookie.Secure {
			t.Errorf("expired cookie: got %+v", cookie)
		}
	}
}

func TestValidOrigin(t *testing.T) {
	for _, origin := range []string{"http://example.test", "https://example.test"} {
		req := httptest.NewRequest(http.MethodPost, "http://example.test/admin/api/login", nil)
		req.Header.Set("Origin", origin)
		if !validOrigin(req) {
			t.Errorf("validOrigin(%q): got false, want true", origin)
		}
	}

	req := httptest.NewRequest(http.MethodPost, "https://example.test/admin/api/login", nil)
	req.Header.Set("Origin", "https://evil.test")
	if validOrigin(req) {
		t.Error("validOrigin(cross-origin): got true, want false")
	}
}

func TestLoginLimiter(t *testing.T) {
	limiter := newLoginLimiter()
	now := time.Now()
	for range 20 {
		if !limiter.allow("client", now) {
			t.Fatal("allow: rejected request before per-client limit")
		}
	}
	if limiter.allow("client", now) {
		t.Fatal("allow: accepted request beyond per-client limit")
	}
	if !limiter.allow("client", now.Add(time.Minute)) {
		t.Fatal("allow: did not reset expired attempts")
	}
}

func TestRemoteHost(t *testing.T) {
	if got := remoteHost("192.0.2.1:1234"); got != "192.0.2.1" {
		t.Errorf("remoteHost: got %q, want %q", got, "192.0.2.1")
	}
	if got := remoteHost("unknown"); got != "unknown" {
		t.Errorf("remoteHost fallback: got %q, want %q", got, "unknown")
	}
}

func TestAdminLoginSession(t *testing.T) {
	sec, err := security.New(security.Config{
		OverrideBaseKeysFolder: t.TempDir(),
		Issuer:                 "test",
	})
	if err != nil {
		t.Fatalf("security.New: %v", err)
	}
	t.Cleanup(func() {
		if err := sec.Close(); err != nil {
			t.Errorf("security.Close: %v", err)
		}
	})

	digest := sha256.Sum256([]byte("secret"))
	app := web.NewApp(func(context.Context, string, ...any) {})
	registerAdminRoutes(app, Config{
		AdminAuthEnabled:    true,
		AdminPasswordSHA256: hex.EncodeToString(digest[:]),
		Security:            sec,
	})
	app.NotFoundHandler()
	handler := adminCookieMiddleware(app)

	sessionReq := httptest.NewRequest(http.MethodGet, "http://example.test/admin/api/session", nil)
	sessionRR := httptest.NewRecorder()
	handler.ServeHTTP(sessionRR, sessionReq)
	if sessionRR.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated session status: got %d, want %d", sessionRR.Code, http.StatusUnauthorized)
	}

	loginBody, err := json.Marshal(loginRequest{Password: "secret"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	loginReq := httptest.NewRequest(http.MethodPost, "http://example.test/admin/api/login", bytes.NewReader(loginBody))
	loginReq.Header.Set("Content-Type", "application/json")
	loginReq.Header.Set("Origin", "http://example.test")
	loginRR := httptest.NewRecorder()
	handler.ServeHTTP(loginRR, loginReq)
	if loginRR.Code != http.StatusOK {
		t.Fatalf("login status: got %d, want %d", loginRR.Code, http.StatusOK)
	}
	cookies := loginRR.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login cookies: got %d, want 1", len(cookies))
	}
	if cookies[0].Name != adminHTTPCookieName || cookies[0].Secure {
		t.Fatalf("HTTP login cookie: got %+v", cookies[0])
	}

	sessionReq = httptest.NewRequest(http.MethodGet, "http://example.test/admin/api/session", nil)
	sessionReq.AddCookie(cookies[0])
	sessionRR = httptest.NewRecorder()
	handler.ServeHTTP(sessionRR, sessionReq)
	if sessionRR.Code != http.StatusOK {
		t.Fatalf("authenticated session status: got %d, want %d", sessionRR.Code, http.StatusOK)
	}
}
