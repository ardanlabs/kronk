// Package show provides the show command code.
package show

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
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

	mi, err := models.RetrieveInfo(modelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model info: %w", err)
	}

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	mp, err := models.RetrievePath(modelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model path: %w", err)
	}

	krn, err := kronk.New(model.Config{
		ModelFiles: mp.ModelFiles,
	})

	if err != nil {
		return err
	}

	printLocal(mi, krn.ModelInfo())

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

	type metaItem struct {
		k string
		v string
	}
	orderedMeta := make([]metaItem, 0, len(mi.Metadata))
	for k, v := range mi.Metadata {
		orderedMeta = append(orderedMeta, metaItem{k: k, v: v})
	}

	slices.SortFunc(orderedMeta, func(a metaItem, b metaItem) int {
		if a.k == b.k {
			return 0
		}
		if a.k > b.k {
			return 1
		}
		return -1
	})

	for _, m := range orderedMeta {
		fmt.Printf("  %s: %s\n", m.k, m.v)
	}
}

func printLocal(mi models.Info, details model.ModelInfo) {
	fmt.Printf("ID:          %s\n", mi.ID)
	fmt.Printf("Object:      %s\n", mi.Object)
	fmt.Printf("Created:     %v\n", time.UnixMilli(mi.Created))
	fmt.Printf("OwnedBy:     %s\n", mi.OwnedBy)
	fmt.Printf("Desc:        %s\n", details.Desc)
	fmt.Printf("Size:        %.2f MiB\n", float64(details.Size)/(1024*1024))
	fmt.Printf("HasProj:     %t\n", details.HasProjection)
	fmt.Printf("HasEncoder:  %t\n", details.HasEncoder)
	fmt.Printf("HasDecoder:  %t\n", details.HasDecoder)
	fmt.Printf("IsRecurrent: %t\n", details.IsRecurrent)
	fmt.Printf("IsHybrid:    %t\n", details.IsHybrid)
	fmt.Printf("IsGPT:       %t\n", details.IsGPTModel)
	fmt.Println("Metadata:")

	type metaItem struct {
		k string
		v string
	}
	orderedMeta := make([]metaItem, 0, len(details.Metadata))
	for k, v := range details.Metadata {
		orderedMeta = append(orderedMeta, metaItem{k: k, v: v})
	}

	slices.SortFunc(orderedMeta, func(a metaItem, b metaItem) int {
		if a.k == b.k {
			return 0
		}
		if a.k > b.k {
			return 1
		}
		return -1
	})

	for _, m := range orderedMeta {
		fmt.Printf("  %s: %s\n", m.k, m.v)
	}
}
