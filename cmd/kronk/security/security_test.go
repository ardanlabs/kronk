package security

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestAuthenticateWebMode(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "server-validates-this-token")

	cmd := &cobra.Command{}
	cmd.Flags().Bool("local", false, "")

	if err := authenticate(cmd, nil); err != nil {
		t.Fatalf("authenticate: got %v, want nil", err)
	}
}

func TestAuthenticateRequiresToken(t *testing.T) {
	t.Setenv("KRONK_TOKEN", "")

	cmd := &cobra.Command{}
	cmd.Flags().Bool("local", false, "")

	if err := authenticate(cmd, nil); err == nil {
		t.Fatal("authenticate: got nil, want error")
	}
}
