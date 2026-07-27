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
