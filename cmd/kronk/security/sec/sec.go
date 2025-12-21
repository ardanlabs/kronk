// Package sec provide a security api for use with the security commands.
package sec

import (
	"context"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/tools/security"
)

var Security *security.Security
var Claims auth.Claims

func init() {
	if (len(os.Args) > 1 && os.Args[1] == "security") ||
		(len(os.Args) > 2 && os.Args[2] == "security") {
		sec, err := security.New(security.Config{
			Issuer: "kronk project",
		})

		if err != nil {
			fmt.Println("not authorized, security init error")
			os.Exit(1)
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
