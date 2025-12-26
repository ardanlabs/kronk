// Package list provides the key list command code.
package list

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/security/sec"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
)

func runWeb() error {
	url, err := client.DefaultURL("/v1/security/keys")
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

	var resp toolapp.KeysResponse

	if err := cln.Do(ctx, http.MethodGet, url, nil, &resp); err != nil {
		return fmt.Errorf("do: unable to list keys: %w", err)
	}

	printKeys(resp)

	return nil
}

func runLocal() error {
	keys, err := sec.Security.ListKeys()
	if err != nil {
		return fmt.Errorf("list-keys: %w", err)
	}

	resp := make(toolapp.KeysResponse, len(keys))
	for i, key := range keys {
		resp[i] = toolapp.KeyResponse{
			ID:      key.ID,
			Created: key.Created.Format(time.RFC3339),
		}
	}

	printKeys(resp)

	return nil
}

func printKeys(keys toolapp.KeysResponse) {
	if len(keys) == 0 {
		fmt.Println("No keys found")
		return
	}

	fmt.Printf("%-40s %s\n", "KEY ID", "CREATED")
	fmt.Println("---------------------------------------- ----------------------------")

	for _, key := range keys {
		fmt.Printf("%-40s %s\n", key.ID, key.Created)
	}
}
