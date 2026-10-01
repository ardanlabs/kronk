package start

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCreateLogFile(t *testing.T) {
	basePath := filepath.Join(t.TempDir(), "missing", "base")

	file, err := createLogFile(basePath)
	if err != nil {
		t.Fatalf("createLogFile: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close log file: %v", err)
	}

	if _, err := os.Stat(filepath.Join(basePath, "kronk.log")); err != nil {
		t.Fatalf("stat log file: %v", err)
	}
}

func TestCreateLogFileError(t *testing.T) {
	basePath := t.TempDir()
	if err := os.Mkdir(filepath.Join(basePath, "kronk.log"), 0o755); err != nil {
		t.Fatalf("make log directory: %v", err)
	}
	if _, err := createLogFile(basePath); err == nil {
		t.Fatal("createLogFile: got nil error, want failure when log path is a directory")
	}
}

func TestLivenessURL(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("api-host", "", "")
	if err := cmd.Flags().Set("api-host", "127.0.0.1:9000"); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := livenessURL(cmd)
	if err != nil {
		t.Fatalf("livenessURL: %v", err)
	}
	if want := "http://127.0.0.1:9000/v1/liveness"; got != want {
		t.Fatalf("livenessURL: got %q, want %q", got, want)
	}
}

func TestReservePIDFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	basePath := t.TempDir()
	file, path, err := reservePIDFile(context.Background(), basePath, server.URL)
	if err != nil {
		t.Fatalf("reservePIDFile: %v", err)
	}
	defer file.Close()
	defer os.Remove(path)

	if _, _, err := reservePIDFile(context.Background(), basePath, server.URL); err == nil || !strings.Contains(err.Error(), "already in progress") {
		t.Fatalf("second reservePIDFile: got %v, want active-start error", err)
	}
}

func TestReservePIDFileRemovesStaleFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	basePath := t.TempDir()
	path := pidFilePath(basePath)
	if err := os.WriteFile(path, []byte("not-a-pid"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if _, _, err := reservePIDFile(context.Background(), basePath, server.URL); err == nil || !strings.Contains(err.Error(), "removed stale pid file") {
		t.Fatalf("reservePIDFile: got %v, want stale-file error", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Stat: got %v, want missing stale pid file", err)
	}
}

func TestBuildEnvVarsAdminPassword(t *testing.T) {
	const value = "test-digest"

	cmd := &cobra.Command{}
	cmd.Flags().String("web-admin-password-sha-256", "", "")
	if err := cmd.Flags().Set("web-admin-password-sha-256", value); err != nil {
		t.Fatalf("Set: %v", err)
	}

	want := "KRONK_WEB_ADMIN_PASSWORD_SHA_256=" + value
	if envVars := buildEnvVars(cmd); !slices.Contains(envVars, want) {
		t.Errorf("buildEnvVars: got %v, want entry %q", envVars, want)
	}
}

func TestBuildEnvVarsInferenceTimeout(t *testing.T) {
	const value = "45m"

	cmd := &cobra.Command{}
	cmd.Flags().String("inference-timeout", "", "")
	if err := cmd.Flags().Set("inference-timeout", value); err != nil {
		t.Fatalf("Set: %v", err)
	}

	want := "KRONK_WEB_INFERENCE_TIMEOUT=" + value
	if envVars := buildEnvVars(cmd); !slices.Contains(envVars, want) {
		t.Errorf("buildEnvVars: got %v, want entry %q", envVars, want)
	}
}

func TestBuildEnvVarsPoolTTL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantEnv bool
	}{
		{name: "omitted"},
		{name: "disabled", value: "0", wantEnv: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("pool-ttl", "", "")
			if tt.value != "" {
				if err := cmd.Flags().Set("pool-ttl", tt.value); err != nil {
					t.Fatalf("Set: %v", err)
				}
			}

			const want = "KRONK_POOL_TTL=0"
			got := slices.Contains(buildEnvVars(cmd), want)
			if got != tt.wantEnv {
				t.Errorf("buildEnvVars contains %q: got %t, want %t", want, got, tt.wantEnv)
			}
		})
	}
}

func TestBuildEnvVarsAuthTLS(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("auth-tls-enabled", false, "")
	cmd.Flags().String("auth-tls-ca-file", "", "")
	cmd.Flags().String("auth-tls-server-name", "", "")

	values := map[string]string{
		"auth-tls-enabled":     "true",
		"auth-tls-ca-file":     "/certs/ca.pem",
		"auth-tls-server-name": "auth.internal",
	}
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
	}

	envVars := buildEnvVars(cmd)
	wants := []string{
		"KRONK_AUTH_TLS_ENABLED=true",
		"KRONK_AUTH_TLS_CA_FILE=/certs/ca.pem",
		"KRONK_AUTH_TLS_SERVER_NAME=auth.internal",
	}
	for _, want := range wants {
		if !slices.Contains(envVars, want) {
			t.Errorf("buildEnvVars: got %v, want entry %q", envVars, want)
		}
	}
}

func TestBuildEnvVarsServiceSettings(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("authorization-mode", "", "")
	cmd.Flags().Bool("download-enabled", false, "")
	cmd.Flags().Bool("lib-download-enabled", true, "")
	cmd.Flags().Bool("media-backends-enabled", true, "")
	cmd.Flags().Bool("lib-verify-enabled", false, "")
	cmd.Flags().String("bucky-lib-path", "", "")
	cmd.Flags().String("malina-lib-path", "", "")

	values := map[string]string{
		"authorization-mode":     "management",
		"download-enabled":       "true",
		"lib-download-enabled":   "false",
		"media-backends-enabled": "false",
		"lib-verify-enabled":     "true",
		"bucky-lib-path":         "/opt/bucky",
		"malina-lib-path":        "/opt/malina",
	}
	for name, value := range values {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("Set %s: %v", name, err)
		}
	}

	envVars := buildEnvVars(cmd)
	wants := []string{
		"KRONK_AUTHORIZATION_MODE=management",
		"KRONK_DOWNLOAD_ENABLED=true",
		"KRONK_LIB_DOWNLOAD_ENABLED=false",
		"KRONK_MEDIA_BACKENDS_ENABLED=false",
		"KRONK_LIB_VERIFY_ENABLED=true",
		"KRONK_BUCKY_LIB_PATH=/opt/bucky",
		"KRONK_MALINA_LIB_PATH=/opt/malina",
	}
	for _, want := range wants {
		if !slices.Contains(envVars, want) {
			t.Errorf("buildEnvVars: got %v, want entry %q", envVars, want)
		}
	}
}

func TestOverrideEnv(t *testing.T) {
	base := []string{"PATH=/bin", "KRONK_WEB_API_HOST=env:9000"}
	overrides := []string{"KRONK_WEB_API_HOST=flag:9001"}

	got := overrideEnv(base, overrides)
	want := "KRONK_WEB_API_HOST=flag:9001"
	if !slices.Contains(got, want) {
		t.Errorf("overrideEnv: got %v, want entry %q", got, want)
	}
}
