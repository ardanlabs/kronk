package start

import (
	"slices"
	"testing"

	"github.com/spf13/cobra"
)

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
	cmd.Flags().Bool("lib-verify-enabled", false, "")
	cmd.Flags().String("bucky-lib-path", "", "")

	values := map[string]string{
		"authorization-mode":   "management",
		"download-enabled":     "true",
		"lib-download-enabled": "false",
		"lib-verify-enabled":   "true",
		"bucky-lib-path":       "/opt/bucky",
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
		"KRONK_LIB_VERIFY_ENABLED=true",
		"KRONK_BUCKY_LIB_PATH=/opt/bucky",
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
