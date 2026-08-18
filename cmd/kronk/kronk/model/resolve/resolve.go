package resolve

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// dropCacheEntry removes the resolver-file entry matching id so the next
// Resolve call hits the HuggingFace API.
func dropCacheEntry(r *models.Resolver, id string) error {
	rm, err := r.Load()
	if err != nil {
		return err
	}

	delete(rm.Models, id)

	return r.Save(rm)
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
