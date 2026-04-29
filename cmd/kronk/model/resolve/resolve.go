package resolve

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// dropCacheEntry removes any resolver-file entries matching id so the next
// Resolve call hits the HuggingFace API. The id may be bare or
// "provider/modelID".
func dropCacheEntry(r *models.Resolver, id string) error {
	rm, err := r.Load()
	if err != nil {
		return err
	}

	provider, modelID := splitProviderID(id)

	for key := range rm.Models {
		keyProvider, keyModel := splitProviderID(key)

		switch {
		case provider != "" && key == id:
			delete(rm.Models, key)
		case provider == "" && keyModel == modelID:
			delete(rm.Models, key)
		default:
			_ = keyProvider
		}
	}

	return r.Save(rm)
}

// splitProviderID separates "provider/modelID" inputs.
func splitProviderID(id string) (provider, modelID string) {
	if before, after, ok := strings.Cut(id, "/"); ok {
		return before, after
	}

	return "", id
}

// printResolution writes a human-readable summary of a Resolution.
func printResolution(rfile string, res models.Resolution) {
	source := "huggingface"
	switch {
	case res.FromLocal:
		source = "local-disk"
	case res.FromCache:
		source = "resolver-file"
	}

	fmt.Println()
	fmt.Println("Model Resolution")
	fmt.Println("================")
	fmt.Printf("Canonical ID:  %s\n", res.CanonicalID)
	fmt.Printf("Provider:      %s\n", res.Provider)
	fmt.Printf("Family:        %s\n", res.Family)
	fmt.Printf("Revision:      %s\n", res.Revision)
	fmt.Printf("Source:        %s\n", source)
	fmt.Printf("Resolver File: %s\n", rfile)

	fmt.Println()
	fmt.Println("Files:")
	for _, f := range res.Files {
		fmt.Printf("  %s\n", f)
	}

	if res.MMProj != "" {
		fmt.Println()
		fmt.Println("Projection (mmproj):")
		fmt.Printf("  %s\n", res.MMProj)
	}

	if len(res.DownloadURLs) > 0 {
		fmt.Println()
		fmt.Println("Download URLs:")
		for _, u := range res.DownloadURLs {
			fmt.Printf("  %s\n", u)
		}
		if res.DownloadProj != "" {
			fmt.Printf("  %s\n", res.DownloadProj)
		}
	}

	if len(res.LocalPaths) > 0 {
		fmt.Println()
		fmt.Println("Local Paths:")
		for _, p := range res.LocalPaths {
			fmt.Printf("  %s\n", p)
		}
		if res.LocalProj != "" {
			fmt.Printf("  %s\n", res.LocalProj)
		}
	}

	fmt.Println()
}
