// Package create provides the token create command code.
package create

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/security/sec"
	"github.com/ardanlabs/kronk/sdk/client"
)

type config struct {
	AdminToken string
	UserName   string
	Endpoints  []string
	Duration   time.Duration
}

func runWeb(cfg config) error {
	fmt.Println("Token create")
	fmt.Printf("  UserName: %s\n", cfg.UserName)
	fmt.Printf("  Duration: %s\n", cfg.Duration)
	fmt.Printf("  Endpoints: %v\n", cfg.Endpoints)

	url, err := client.DefaultURL("/v1/security/token/create")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	req := client.D{
		"user_name": cfg.UserName,
		"admin":     false,
		"endpoints": cfg.Endpoints,
		"duration":  cfg.Duration,
	}

	c := client.New(client.FmtLogger, client.WithBearer(cfg.AdminToken))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var resp struct {
		Token string `json:"token"`
	}
	if err := c.Do(ctx, http.MethodPost, url, req, &resp); err != nil {
		return fmt.Errorf("do: unable to create token: %w", err)
	}

	fmt.Println("TOKEN:")
	fmt.Println(resp.Token)

	return nil
}

func runLocal(cfg config) error {
	fmt.Println("Token create")
	fmt.Printf("  UserName: %s\n", cfg.UserName)
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
