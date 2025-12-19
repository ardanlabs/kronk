// Package create provides the key create command code.
package create

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/security/sec"
	"github.com/ardanlabs/kronk/sdk/client"
)

func runWeb() error {
	url, err := client.DefaultURL("/v1/security/keys/add")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	adminToken := os.Getenv("KRONK_TOKEN")

	c := client.New(client.FmtLogger, client.WithBearer(adminToken))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := c.Do(ctx, http.MethodPost, url, nil, nil); err != nil {
		return fmt.Errorf("do: unable to create key: %w", err)
	}

	fmt.Println("Private key created successfully")

	return nil
}

func runLocal() error {
	if err := sec.Security.AddPrivateKey(); err != nil {
		return fmt.Errorf("add-private-key: %w", err)
	}

	fmt.Println("Private key created successfully")

	return nil
}
