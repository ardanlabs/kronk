// Package apitest provides support for excuting api test logic.
package apitest

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
)

type testOption struct {
	skip    bool
	skipMsg string
}

// OptionFunc represents a function that can set options.
type OptionFunc func(*testOption)

// WithSkip can be used to skip running a test.
func WithSkip(skip bool, msg string) OptionFunc {
	return func(to *testOption) {
		to.skip = skip
		to.skipMsg = msg
	}
}

// Test contains functions for executing an api test.
type Test struct {
	Sec *security.Security
	mux http.Handler
}

// Run performs the actual test logic based on the table data.
func (at *Test) Run(t *testing.T, table []Table, testName string, options ...OptionFunc) {
	to := new(testOption)
	for _, f := range options {
		f(to)
	}

	if to.skip {
		t.Skipf("%v: %v", testName, to.skipMsg)
	}

	for _, tt := range table {
		f := func(t *testing.T) {
			if tt.SkipInGH && os.Getenv("GITHUB_ACTIONS") == "true" {
				t.Skip("Skipping test in GitHub Actions")
			}

			r := httptest.NewRequest(tt.Method, tt.URL, nil)
			w := httptest.NewRecorder()

			if tt.Input != nil {
				d, err := json.Marshal(tt.Input)
				if err != nil {
					t.Fatalf("Should be able to marshal the model : %s", err)
				}

				r = httptest.NewRequest(tt.Method, tt.URL, bytes.NewBuffer(d))
			}

			r.Header.Set("Authorization", "Bearer "+tt.Token)
			at.mux.ServeHTTP(w, r)

			if w.Code != tt.StatusCode {
				t.Fatalf("%s: Should receive a status code of %d for the response : %d", tt.Name, tt.StatusCode, w.Code)
			}

			if tt.StatusCode == http.StatusNoContent {
				return
			}

			if err := json.Unmarshal(w.Body.Bytes(), tt.GotResp); err != nil {
				t.Fatalf("Should be able to unmarshal the response : %s", err)
			}

			diff := tt.CmpFunc(tt.GotResp, tt.ExpResp)
			if diff != "" {
				t.Log("DIFF")
				t.Logf("%s", diff)
				t.Log("GOT")
				t.Logf("%#v", tt.GotResp)
				t.Log("EXP")
				t.Logf("%#v", tt.ExpResp)
				t.Fatalf("Should get the expected response")
			}
		}

		t.Run(testName+"-"+tt.Name, f)
	}
}

// RunStreaming performs the actual test logic based on the table data.
// It parses SSE events and tests only the last data event (ignoring deltas).
func (at *Test) RunStreaming(t *testing.T, table []Table, testName string, options ...OptionFunc) {
	to := new(testOption)
	for _, f := range options {
		f(to)
	}

	if to.skip {
		t.Skipf("%v: %v", testName, to.skipMsg)
	}

	for _, tt := range table {
		f := func(t *testing.T) {
			r := httptest.NewRequest(tt.Method, tt.URL, nil)
			w := httptest.NewRecorder()

			if tt.Input != nil {
				d, err := json.Marshal(tt.Input)
				if err != nil {
					t.Fatalf("Should be able to marshal the model : %s", err)
				}

				r = httptest.NewRequest(tt.Method, tt.URL, bytes.NewBuffer(d))
			}

			r.Header.Set("Authorization", "Bearer "+tt.Token)
			at.mux.ServeHTTP(w, r)

			if w.Code != tt.StatusCode {
				t.Fatalf("%s: Should receive a status code of %d for the response : %d", tt.Name, tt.StatusCode, w.Code)
			}

			if tt.StatusCode == http.StatusNoContent {
				return
			}

			var lastData string
			scanner := bufio.NewScanner(w.Body)
			for scanner.Scan() {
				line := scanner.Text()
				if after, ok := strings.CutPrefix(line, "data: "); ok {
					data := after
					if data != "[DONE]" {
						lastData = data
					}
				}
			}

			if lastData == "" {
				t.Fatalf("Should have received at least one SSE data event")
			}

			if err := json.Unmarshal([]byte(lastData), tt.GotResp); err != nil {
				t.Fatalf("Should be able to unmarshal the response : %s", err)
			}

			diff := tt.CmpFunc(tt.GotResp, tt.ExpResp)
			if diff != "" {
				t.Log("DIFF")
				t.Logf("%s", diff)
				t.Log("GOT")
				t.Logf("%#v", tt.GotResp)
				t.Log("EXP")
				t.Logf("%#v", tt.ExpResp)
				t.Fatalf("Should get the expected response")
			}
		}

		t.Run(testName+"-"+tt.Name, f)
	}
}

// =============================================================================

// Token generates an authenticated token for a user.
func Token(ath *auth.Auth, admin bool, endpoints map[string]auth.RateLimit) string {
	claims := auth.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uuid.NewString(),
			Issuer:    ath.Issuer(),
			ExpiresAt: jwt.NewNumericDate(time.Now().UTC().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now().UTC()),
		},
		Admin:     admin,
		Endpoints: endpoints,
	}

	token, err := ath.GenerateToken(claims)
	if err != nil {
		return ""
	}

	return token
}
