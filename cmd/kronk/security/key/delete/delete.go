// Package delete provides the key delete command code.
package delete

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/security/sec"
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

	cln := client.New(
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := cln.Do(ctx, http.MethodPost, url, nil, nil); err != nil {
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
