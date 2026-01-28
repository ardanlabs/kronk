// Package show provides the show command code.
package show

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb(args []string) error {
	modelID := args[0]

	url, err := client.DefaultURL(fmt.Sprintf("/v1/models/%s", modelID))
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

	var info toolapp.ModelInfoResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &info); err != nil {
		return fmt.Errorf("do: unable to get model information: %w", err)
	}

	printWeb(info)

	return nil
}

func runLocal(models *models.Models, args []string) error {
	modelID := args[0]

	fsModelID, _, _ := strings.Cut(modelID, "/")

	fi, err := models.RetrieveInfo(fsModelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model file info: %w", err)
	}
	fi.ID = modelID

	mi, err := models.RetrieveModelInfo(fsModelID)
	if err != nil {
		return errs.New(errs.Internal, err)
	}

	printLocal(fi, mi)

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
	fmt.Printf("IsGPT:       %t\n", mi.IsGPT)
	fmt.Println("Metadata:")
	for k, v := range mi.Metadata {
		fmt.Printf("  %s: %s\n", k, v)
	}
}

func printLocal(fi models.FileInfo, mi models.ModelInfo) {
	fmt.Printf("ID:          %s\n", fi.ID)
	fmt.Printf("Object:      %s\n", fi.Object)
	fmt.Printf("Created:     %v\n", time.UnixMilli(fi.Created))
	fmt.Printf("OwnedBy:     %s\n", fi.OwnedBy)
	fmt.Printf("Desc:        %s\n", mi.Desc)
	fmt.Printf("Size:        %.2f MiB\n", float64(mi.Size)/(1024*1024))
	fmt.Printf("HasProj:     %t\n", mi.HasProjection)
	fmt.Printf("IsGPT:       %t\n", mi.IsGPTModel)
	fmt.Printf("IsEmbed:     %t\n", mi.IsEmbedModel)
	fmt.Printf("IsRerank:    %t\n", mi.IsRerankModel)
	fmt.Println("Metadata:")
	for k, v := range mi.Metadata {
		fmt.Printf("  %s: %s\n", k, formatMetadataValue(v))
	}
}

func formatMetadataValue(value string) string {
	if len(value) < 2 || value[0] != '[' {
		return value
	}

	inner := value[1 : len(value)-1]
	elements := strings.Split(inner, " ")

	if len(elements) <= 6 {
		return value
	}

	first := elements[:3]

	return fmt.Sprintf("[%s, ...]", strings.Join(first, ", "))
}
