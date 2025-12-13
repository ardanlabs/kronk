// Package list provides the catalog list command code.
package list

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
	"github.com/ardanlabs/kronk/tools/catalog"
)

// RunWeb executes the catalog list command against the model server.
func RunWeb(args []string) error {
	fmt.Println("catalog list: not implemented")
	return nil
}

// RunLocal executes the catalog list command locally.
func RunLocal(args []string) error {
	basePath := defaults.BaseDir("")

	rows, pulledModels, err := catalogList(basePath)
	if err != nil {
		return fmt.Errorf("catalog-list: %w", err)
	}

	print(rows, pulledModels)

	return nil
}

// =============================================================================

type row struct {
	catalogName string
	model       catalog.Model
}

func catalogList(basePath string) ([]row, map[string]struct{}, error) {
	catalogs, err := catalog.RetrieveCatalogs(basePath)
	if err != nil {
		return nil, nil, fmt.Errorf("catalog list: %w", err)
	}

	modelBasePath := defaults.ModelsDir("")

	modelFiles, err := tools.RetrieveModelFiles(modelBasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("retrieve-model-files: %w", err)
	}

	pulledModels := make(map[string]struct{})
	for _, mf := range modelFiles {
		pulledModels[strings.ToLower(mf.ID)] = struct{}{}
	}

	var rows []row
	for _, cat := range catalogs {
		for _, model := range cat.Models {
			rows = append(rows, row{catalogName: cat.Name, model: model})
		}
	}

	slices.SortFunc(rows, func(a, b row) int {
		if c := cmp.Compare(strings.ToLower(a.catalogName), strings.ToLower(b.catalogName)); c != 0 {
			return c
		}
		return cmp.Compare(strings.ToLower(a.model.ID), strings.ToLower(b.model.ID))
	})

	return rows, pulledModels, nil
}

func print(rows []row, pulledModels map[string]struct{}) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "CATALOG\tMODEL ID\tPULLED\tENDPOINT\tIMAGES\tAUDIO\tVIDEO\tSTREAMING\tREASONING\tTOOLING")

	for _, r := range rows {
		_, onDisk := pulledModels[strings.ToLower(r.model.ID)]

		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.catalogName,
			r.model.ID,
			boolToStr(onDisk),
			r.model.Capabilities.Endpoint,
			boolToStr(r.model.Capabilities.Images),
			boolToStr(r.model.Capabilities.Audio),
			boolToStr(r.model.Capabilities.Video),
			boolToStr(r.model.Capabilities.Streaming),
			boolToStr(r.model.Capabilities.Reasoning),
			boolToStr(r.model.Capabilities.Tooling),
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
