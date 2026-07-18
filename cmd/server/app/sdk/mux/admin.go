package mux

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
)

const adminCookieName = "__Host-kronk-admin"

type loginRequest struct {
	Password string `json:"password"`
}

type loginLimiter struct {
	mu       sync.Mutex
	global   []time.Time
	byRemote map[string][]time.Time
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{byRemote: make(map[string][]time.Time)}
}

func (ll *loginLimiter) allow(remote string, now time.Time) bool {
	ll.mu.Lock()
	defer ll.mu.Unlock()

	cutoff := now.Add(-time.Minute)
	ll.global = recentAttempts(ll.global, cutoff)
	if len(ll.byRemote) > 1000 {
		for name, attempts := range ll.byRemote {
			attempts = recentAttempts(attempts, cutoff)
			if len(attempts) == 0 {
				delete(ll.byRemote, name)
				continue
			}
			ll.byRemote[name] = attempts
		}
	}
	remoteAttempts := recentAttempts(ll.byRemote[remote], cutoff)
	if len(ll.global) >= 100 || len(remoteAttempts) >= 20 {
		ll.byRemote[remote] = remoteAttempts
		return false
	}

	ll.global = append(ll.global, now)
	ll.byRemote[remote] = append(remoteAttempts, now)
	return true
}

func recentAttempts(attempts []time.Time, cutoff time.Time) []time.Time {
	for i, attempt := range attempts {
		if attempt.After(cutoff) {
			return attempts[i:]
		}
	}
	return nil
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err == nil {
		return host
	}
	return remoteAddr
}

func registerAdminRoutes(app *web.App, cfg Config) {
	limiter := newLoginLimiter()

	app.RawHandlerFunc(http.MethodGet, "", "/admin", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/", http.StatusPermanentRedirect)
	})

	app.RawHandlerFunc(http.MethodPost, "admin", "/api/login", func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		if !cfg.AdminAuthEnabled {
			writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true})
			return
		}
		if !limiter.allow(remoteHost(r.RemoteAddr), time.Now()) {
			http.Error(w, "invalid credentials", http.StatusTooManyRequests)
			return
		}
		if !validOrigin(r) || !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		var input loginRequest
		if err := dec.Decode(&input); err != nil || len(input.Password) > 1024 {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		configured, err := hex.DecodeString(cfg.AdminPasswordSHA256)
		digest := sha256.Sum256([]byte(input.Password))
		if err != nil || subtle.ConstantTimeCompare(digest[:], configured) != 1 {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		token, err := cfg.Security.GenerateToken(true, nil, time.Hour)
		if err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}

		setAdminCookie(w, token, time.Now().Add(time.Hour), 3600)
		writeJSON(w, http.StatusOK, sessionResponse{Authenticated: true, AuthenticationRequired: true})
	})

	app.RawHandlerFunc(http.MethodGet, "admin", "/api/session", func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		authenticated := !cfg.AdminAuthEnabled
		if cfg.AdminAuthEnabled {
			if cookie, err := r.Cookie(adminCookieName); err == nil {
				_, err = cfg.Security.Authenticate(r.Context(), "Bearer "+cookie.Value, true, "")
				authenticated = err == nil
			}
		}
		status := http.StatusOK
		if !authenticated {
			status = http.StatusUnauthorized
		}
		writeJSON(w, status, sessionResponse{Authenticated: authenticated, AuthenticationRequired: cfg.AdminAuthEnabled})
	})

	app.RawHandlerFunc(http.MethodPost, "admin", "/api/logout", func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		setAdminCookie(w, "", time.Unix(1, 0), -1)
		writeJSON(w, http.StatusOK, sessionResponse{AuthenticationRequired: cfg.AdminAuthEnabled})
	})
}

type sessionResponse struct {
	Authenticated          bool `json:"authenticated"`
	AuthenticationRequired bool `json:"authentication_required"`
}

func adminCookieMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/admin/") {
			securityHeaders(w)
		}

		if r.Header.Get("Authorization") == "" {
			if cookie, err := r.Cookie(adminCookieName); err == nil {
				if !isSafeMethod(r.Method) && !validOrigin(r) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				r.Header.Set("Authorization", "Bearer "+cookie.Value)
			}
		}

		next.ServeHTTP(w, r)
	})
}

func setAdminCookie(w http.ResponseWriter, value string, expires time.Time, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: adminCookieName, Value: value, Path: "/", Expires: expires,
		MaxAge: maxAge, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

func validOrigin(r *http.Request) bool {
	origin, err := url.Parse(r.Header.Get("Origin"))
	if err != nil {
		return false
	}
	return (origin.Scheme == "http" || origin.Scheme == "https") && origin.Host == r.Host
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func securityHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; img-src 'self' https: data:; style-src 'self' 'unsafe-inline'; object-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
