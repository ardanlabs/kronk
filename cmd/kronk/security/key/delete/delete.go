// Package delete provides the key delete command code.
package delete

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/security/sec"
	"github.com/ardanlabs/kronk/sdk/client"
)

const (
	masterFile = "master"
)

func runWeb(keyID string) error {
	url, err := client.DefaultURL("/v1/security/keys/remove/" + keyID)
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	adminToken := os.Getenv("KRONK_TOKEN")

	c := client.New(client.FmtLogger, client.WithBearer(adminToken))

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := c.Do(ctx, http.MethodPost, url, nil, nil); err != nil {
		return fmt.Errorf("do: unable to delete key: %w", err)
	}

	fmt.Printf("Private key %q deleted successfully\n", keyID)

	return nil
}

func runLocal(keyID string) error {
	if keyID == masterFile {
		return fmt.Errorf("cannot delete the master key")
	}

	if err := sec.Security.DeletePrivateKey(keyID); err != nil {
		return fmt.Errorf("delete-private-key: %w", err)
	}

	fmt.Printf("Private key %q deleted successfully\n", keyID)

	return nil
}
