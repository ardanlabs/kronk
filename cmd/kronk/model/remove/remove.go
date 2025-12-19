// Package remove provides the remove command code.
package remove

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/sdk/client"
	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
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

	client := client.New(client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if err := client.Do(ctx, http.MethodDelete, url, nil, nil); err != nil {
		return fmt.Errorf("remove-model: %w", err)
	}

	fmt.Println("Remove complete")

	return nil
}

func runLocal(args []string) error {
	modelPath := defaults.ModelsDir("")
	modelID := args[0]

	fmt.Println("Model Path: ", modelPath)
	fmt.Println("Model ID  : ", modelID)

	mp, err := models.RetrievePath(modelPath, modelID)
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

	if err := models.Remove(mp); err != nil {
		return fmt.Errorf("remove-model: %w", err)
	}

	fmt.Println("Remove complete")

	return nil
}
