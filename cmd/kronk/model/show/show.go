// Package show provides the show command code.
package show

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/kronk/website/app/domain/toolapp"
	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

// RunWeb executes the show command against the model server.
func RunWeb(args []string) error {
	modelID := args[0]

	url, err := client.DefaultURL(fmt.Sprintf("/v1/models/%s", modelID))
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	client := client.New(client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var info toolapp.ModelInfoResponse
	if err := client.Do(ctx, http.MethodGet, url, nil, &info); err != nil {
		return fmt.Errorf("do: unable to get mode information: %w", err)
	}

	printWeb(info)

	return nil
}

// RunLocal executes the pull command.
func RunLocal(args []string) error {
	libPath := defaults.LibsDir("")
	modelBasePath := defaults.ModelsDir("")
	modelID := args[0]

	mi, err := tools.RetrieveModelInfo(libPath, modelBasePath, modelID)
	if err != nil {
		return err
	}

	printLocal(mi)

	return nil
}

// =============================================================================

func printWeb(mi toolapp.ModelInfoResponse) {
	fmt.Printf("ID:          %s\n", mi.ID)
	fmt.Printf("Object:      %s\n", mi.Object)
	fmt.Printf("Created:     %v\n", time.UnixMilli(mi.Created))
	fmt.Printf("OwnedBy:     %s\n", mi.OwnedBy)
	fmt.Printf("Desc:        %s\n", mi.Desc)
	fmt.Printf("Size:        %.2f MiB\n", float64(mi.Size)/(1024*1024))
	fmt.Printf("HasProj:     %t\n", mi.HasProjection)
	fmt.Printf("HasEncoder:  %t\n", mi.HasEncoder)
	fmt.Printf("HasDecoder:  %t\n", mi.HasDecoder)
	fmt.Printf("IsRecurrent: %t\n", mi.IsRecurrent)
	fmt.Printf("IsHybrid:    %t\n", mi.IsHybrid)
	fmt.Printf("IsGPT:       %t\n", mi.IsGPT)
	fmt.Println("Metadata:")
	for k, v := range mi.Metadata {
		fmt.Printf("  %s: %s\n", k, v)
	}
}

func printLocal(mi tools.ModelInfo) {
	fmt.Printf("ID:          %s\n", mi.ID)
	fmt.Printf("Object:      %s\n", mi.Object)
	fmt.Printf("Created:     %v\n", time.UnixMilli(mi.Created))
	fmt.Printf("OwnedBy:     %s\n", mi.OwnedBy)
	fmt.Printf("Desc:        %s\n", mi.Details.Desc)
	fmt.Printf("Size:        %.2f MiB\n", float64(mi.Details.Size)/(1024*1024))
	fmt.Printf("HasProj:     %t\n", mi.Details.HasProjection)
	fmt.Printf("HasEncoder:  %t\n", mi.Details.HasEncoder)
	fmt.Printf("HasDecoder:  %t\n", mi.Details.HasDecoder)
	fmt.Printf("IsRecurrent: %t\n", mi.Details.IsRecurrent)
	fmt.Printf("IsHybrid:    %t\n", mi.Details.IsHybrid)
	fmt.Printf("IsGPT:       %t\n", mi.Details.IsGPTModel)
	fmt.Println("Metadata:")
	for k, v := range mi.Details.Metadata {
		fmt.Printf("  %s: %s\n", k, v)
	}
}
