// Package security provides tooling support for security.
package security

import (
	"context"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/security/token"
	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/tools/security"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "security",
	Short: "Manage security",
	Long: `Manage security - tokens and access control

Environment Variables:
  KRONK_TOKEN    Admin level token required for authentication. Must be set
                 before running any security commands.`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(token.Cmd)
}

var Security *security.Security
var Claims auth.Claims

// =============================================================================

func init() {
	if (len(os.Args) > 1 && os.Args[1] == "security") ||
		(len(os.Args) > 2 && os.Args[2] == "security") {
		sec, err := security.New(security.Config{
			Issuer:  "kronk project",
			Enabled: true,
		})

		if err != nil {
			fmt.Println("not authorized, security init error")
			os.Exit(1)
		}

		if os.Getenv("KRONK_TOKEN") == "" {
			Cmd.Help()
			os.Exit(0)
		}

		ctx := context.Background()
		bearerToken := fmt.Sprintf("Bearer %s", os.Getenv("KRONK_TOKEN"))
		claims, err := sec.Auth.Authenticate(ctx, bearerToken)
		if err != nil {
			fmt.Println("\nNOT AUTHORIZED, Invalid Token")
			os.Exit(1)
		}

		if !claims.Admin {
			fmt.Println("\nNOT AUTHORIZED, NOT Admin")
			os.Exit(1)
		}

		Security = sec
		Claims = claims
	}
}
