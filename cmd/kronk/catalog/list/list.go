// Package list provides the catalog list command code.
package list

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"text/tabwriter"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/client"
	"github.com/ardanlabs/kronk/sdk/kronk/defaults"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
)

func runWeb(args []string) error {
	url, err := client.DefaultURL("/v1/catalog")
	if err != nil {
		return fmt.Errorf("default-url: %w", err)
	}

	fmt.Println("URL:", url)

	client := client.New(client.FmtLogger)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	var list []toolapp.CatalogModelResponse
	if err := client.Do(ctx, http.MethodGet, url, nil, &list); err != nil {
		return fmt.Errorf("do: unable to get model list: %w", err)
	}

	printWeb(list)

	return nil
}

func runLocal(args []string) error {
	var filterCategory string

	fs := flag.NewFlagSet("catalog list", flag.ContinueOnError)
	fs.StringVar(&filterCategory, "filter-category", "", "filter catalogs by category name (substring match)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	basePath := defaults.BaseDir("")

	list, err := catalog.CatalogModelList(basePath, filterCategory)
	if err != nil {
		return fmt.Errorf("catalog-list: %w", err)
	}

	print(list)

	return nil
}

// =============================================================================

func printWeb(list []toolapp.CatalogModelResponse) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "CATALOG\tMODEL ID\tPULLED\tENDPOINT\tIMAGES\tAUDIO\tVIDEO\tSTREAMING\tREASONING\tTOOLING")

	for _, m := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			m.Category,
			m.ID,
			boolToStr(m.Downloaded),
			m.Capabilities.Endpoint,
			boolToStr(m.Capabilities.Images),
			boolToStr(m.Capabilities.Audio),
			boolToStr(m.Capabilities.Video),
			boolToStr(m.Capabilities.Streaming),
			boolToStr(m.Capabilities.Reasoning),
			boolToStr(m.Capabilities.Tooling),
		)
	}

	w.Flush()
}

func print(list []catalog.Model) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "CATALOG\tMODEL ID\tPULLED\tENDPOINT\tIMAGES\tAUDIO\tVIDEO\tSTREAMING\tREASONING\tTOOLING")

	for _, m := range list {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			m.Category,
			m.ID,
			boolToStr(m.Downloaded),
			m.Capabilities.Endpoint,
			boolToStr(m.Capabilities.Images),
			boolToStr(m.Capabilities.Audio),
			boolToStr(m.Capabilities.Video),
			boolToStr(m.Capabilities.Streaming),
			boolToStr(m.Capabilities.Reasoning),
			boolToStr(m.Capabilities.Tooling),
		)
	}

	w.Flush()
}

func boolToStr(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
