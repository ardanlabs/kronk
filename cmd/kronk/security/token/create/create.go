// Package create provides the token create command code.
package create

import (
	"fmt"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/security/sec"
)

type config struct {
	AdminToken string
	UserName   string
	Endpoints  map[string]bool
	Duration   time.Duration
}

func runWeb(cfg config) error {
	fmt.Println("RunWeb: token create")
	fmt.Printf("  AdminToken: %s\n", cfg.AdminToken)
	fmt.Printf("  Duration: %s\n", cfg.Duration)
	fmt.Printf("  Endpoints: %v\n", cfg.Endpoints)

	return nil
}

func runLocal(cfg config) error {
	fmt.Println("RunLocal: token create")
	fmt.Printf("  Duration: %s\n", cfg.Duration)
	fmt.Printf("  Endpoints: %v\n", cfg.Endpoints)

	token, err := sec.Security.GenerateToken(cfg.UserName, false, cfg.Endpoints, cfg.Duration)
	if err != nil {
		return fmt.Errorf("generate-token: %w", err)
	}

	fmt.Println("TOKEN:")
	fmt.Println(token)

	return nil
}
