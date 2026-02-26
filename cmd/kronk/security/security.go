// Package security provides tooling support for security.
package security

import (
	"errors"
	"os"

	"github.com/ardanlabs/kronk/cmd/kronk/security/key"
	"github.com/ardanlabs/kronk/cmd/kronk/security/sec"
	"github.com/ardanlabs/kronk/cmd/kronk/security/token"
	"github.com/spf13/cobra"
)

var Cmd = &cobra.Command{
	Use:   "security",
	Short: "Manage API security (keys and tokens)",
	Long: `Manage API security - create, list, and revoke private keys and JWT tokens.

The security command provides access control for the Kronk Model Server through
JWT-based authentication. It manages two types of credentials:

PRIVATE KEYS
  Used to sign JWT tokens. Each key has a unique ID and is used for token
  issuance. Keys can be created, listed, and revoked.

JWT TOKENS
  Short-lived credentials issued by private keys. Tokens are used to authenticate
  API requests and can include custom claims for fine-grained authorization.

REQUIREMENTS

  This command requires an admin-level token to be set via the KRONK_TOKEN
  environment variable before execution.

COMMANDS

  key     Manage private keys (create, list, delete)
  token   Manage JWT tokens (create)

ENVIRONMENT VARIABLES

  KRONK_TOKEN    Admin-level token required for authentication. Must be set
                 before running any security commands.

EXAMPLES

  # Set admin token and list keys
  export KRONK_TOKEN=<admin-token>
  kronk security key list

  # Create a new private key
  kronk security key create --name=my-key

  # Create a JWT token for a user
  kronk security token create --user=john --ttl=1h`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if os.Getenv("KRONK_TOKEN") == "" {
			return errors.New("KRONK_TOKEN environment variable must be set")
		}
		return sec.Authenticate()
	},
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	Cmd.AddCommand(key.Cmd)
	Cmd.AddCommand(token.Cmd)
}
