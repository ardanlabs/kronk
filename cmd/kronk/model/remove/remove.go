// Package remove provides the remove command code.
package remove

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb(args []string) error {
	url, err := client.DefaultURL("/v1/models")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	url = fmt.Sprintf("%s/%s", url, args[0])

	fmt.Println("URL:", url)

	fmt.Printf("\nAre you sure you want to remove %q? (y/n): ", args[0])

	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		fmt.Println("Remove cancelled")
		return nil
	}

	cln := client.New(
		client.FmtLogger,
		client.WithBearer(os.Getenv("KRONK_TOKEN")),
	)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := cln.Do(ctx, http.MethodDelete, url, nil, nil); err != nil {
		return fmt.Errorf("remove-model: %w", err)
	}

	fmt.Println("Remove complete")

	return nil
}

func runLocal(models *models.Models, args []string) error {
	modelID := args[0]

	fmt.Println("Model Path: ", models.Path())
	fmt.Println("Model ID  : ", modelID)

	mp, err := models.RetrievePath(modelID)
	if err != nil {
		return err
	}

	fmt.Printf("\nAre you sure you want to remove %q? (y/n): ", modelID)

	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" {
		fmt.Println("Remove cancelled")
		return nil
	}

	if err := models.Remove(mp, kronk.FmtLogger); err != nil {
		return fmt.Errorf("remove-model: %w", err)
	}

	fmt.Println("Remove complete")

	return nil
}
