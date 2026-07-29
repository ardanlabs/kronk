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

func TestBuildEnvVarsSessions(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("sessions-enabled", false, "")
	cmd.Flags().Int("sessions-max-completed", 0, "")
	if err := cmd.Flags().Set("sessions-enabled", "true"); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if err := cmd.Flags().Set("sessions-max-completed", "2500"); err != nil {
		t.Fatalf("set max completed: %v", err)
	}

	envVars := buildEnvVars(cmd)
	for _, want := range []string{
		"KRONK_SESSIONS_ENABLED=true",
		"KRONK_SESSIONS_MAX_COMPLETED=2500",
	} {
		if !slices.Contains(envVars, want) {
			t.Errorf("buildEnvVars: got %v, want entry %q", envVars, want)
		}
	}
}
