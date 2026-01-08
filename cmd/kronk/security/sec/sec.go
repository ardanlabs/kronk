// Package sec provide a security api for use with the security commands.
package sec

import (
	"context"
	"fmt"
	"os"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security"
)

var Security *security.Security

// Authenticate initializes the security system and authenticates using KRONK_TOKEN.
func Authenticate() error {
	sec, err := security.New(security.Config{
		Issuer: "kronk project",
	})
	if err != nil {
		return fmt.Errorf("security init error: %w", err)
	}

	ctx := context.Background()
	bearerToken := fmt.Sprintf("Bearer %s", os.Getenv("KRONK_TOKEN"))

	if _, err := sec.Authenticate(ctx, bearerToken, true, ""); err != nil {
		sec.Close()
		return fmt.Errorf("not authorized: %w", err)
	}

	Security = sec

	return nil
}
