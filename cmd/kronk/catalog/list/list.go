// Package list provides the catalog list command code.
package list

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"sort"
	"text/tabwriter"
	"time"

	"github.com/ardanlabs/kronk/cmd/kronk/client"
	"github.com/ardanlabs/kronk/cmd/server/app/domain/toolapp"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func runWeb() error {
	url, err := client.DefaultURL("/v1/catalog")
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

	var summaries toolapp.CatalogListResponse
	if err := cln.Do(ctx, http.MethodGet, url, nil, &summaries); err != nil {
		return fmt.Errorf("do: unable to get catalog list: %w", err)
	}

	print(summaries)

	return nil
}

func runLocal(mdls *models.Models) error {
	cat, err := mdls.Catalog()
	if err != nil {
		return fmt.Errorf("load catalog: %w", err)
	}

	downloaded, validated := mdls.IndexState()

	summaries := make([]models.CatalogSummary, 0, len(cat.Models))
	for canonical, entry := range cat.Models {
		summaries = append(summaries, models.NewSummary(canonical, entry, downloaded, validated))
	}

	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].ID < summaries[j].ID
	})

	print(summaries)

	return nil
}

// =============================================================================

func print(summaries []models.CatalogSummary) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tOWNED BY\tFAMILY\tSIZE\tDL\tVAL")

	for _, s := range summaries {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			s.ID, s.OwnedBy, s.ModelFamily, s.TotalSize, yesNo(s.Downloaded), yesNo(s.Validated))
	}

	w.Flush()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
