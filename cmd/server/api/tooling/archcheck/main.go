// This program walks through all models in the catalog, loads each
// downloaded model to determine its architecture type (Dense, MoE, or
// Hybrid) using detectModelType, and compares it against what the catalog
// says. Use -update to write corrected architecture values back to catalog
// files.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/ardanlabs/kronk/sdk/tools/models"
	"go.yaml.in/yaml/v2"
)

func main() {
	if err := run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func run() error {
	var catalogRepoPath string
	var update bool
	var modelID string

	flag.StringVar(&catalogRepoPath, "catalog-path", "", "path to the catalog repo catalogs directory (for updates)")
	flag.BoolVar(&update, "update", false, "update catalog files with corrected architecture values")
	flag.StringVar(&modelID, "model", "", "check a single model by ID instead of all models")
	flag.Parse()

	if update && catalogRepoPath == "" {
		return fmt.Errorf("--catalog-path is required when using --update")
	}

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("initializing backend: %w", err)
	}

	cat, err := catalog.New()
	if err != nil {
		return fmt.Errorf("creating catalog: %w", err)
	}

	mdls, err := models.New()
	if err != nil {
		return fmt.Errorf("creating models: %w", err)
	}

	var allModels []catalog.ModelDetails

	switch modelID {
	case "":
		allModels, err = cat.ModelList("")
		if err != nil {
			return fmt.Errorf("listing catalog models: %w", err)
		}

	default:
		md, err := cat.Details(modelID)
		if err != nil {
			return fmt.Errorf("model %q not found in catalog: %w", modelID, err)
		}
		allModels = []catalog.ModelDetails{md}
	}

	type result struct {
		modelID      string
		catalogArch  string
		ggufArch     string
		detectedType string
		match        bool
		downloaded   bool
	}

	var results []result
	var mismatches int

	for _, m := range allModels {
		mp, err := mdls.FullPath(m.ID)
		if err != nil {
			results = append(results, result{
				modelID:     m.ID,
				catalogArch: m.Architecture,
				downloaded:  false,
			})
			continue
		}

		fmt.Printf("Loading %s ...\n", m.ID)

		mt, ggufArch, err := model.DetectModelTypeFromFiles(mp.ModelFiles)
		if err != nil {
			fmt.Printf("  ERROR: %v\n", err)
			results = append(results, result{
				modelID:     m.ID,
				catalogArch: m.Architecture,
				downloaded:  false,
			})
			continue
		}

		detectedType := modelTypeToArch(mt)

		match := strings.EqualFold(m.Architecture, detectedType)
		if !match {
			mismatches++
		}

		results = append(results, result{
			modelID:      m.ID,
			catalogArch:  m.Architecture,
			ggufArch:     ggufArch,
			detectedType: detectedType,
			match:        match,
			downloaded:   true,
		})
	}

	// -------------------------------------------------------------------------
	// Print results.

	fmt.Println()
	fmt.Printf("%-50s %-12s %-15s %-12s %s\n", "MODEL", "CATALOG", "GGUF ARCH", "DETECTED", "STATUS")
	fmt.Println(strings.Repeat("-", 100))

	for _, r := range results {
		if !r.downloaded {
			fmt.Printf("%-50s %-12s %-15s %-12s %s\n", r.modelID, r.catalogArch, "-", "-", "NOT DOWNLOADED")
			continue
		}

		status := "OK"
		if !r.match {
			status = fmt.Sprintf("MISMATCH (should be %s)", r.detectedType)
		}

		fmt.Printf("%-50s %-12s %-15s %-12s %s\n", r.modelID, r.catalogArch, r.ggufArch, r.detectedType, status)
	}

	fmt.Println()
	fmt.Printf("Total: %d models, %d mismatches\n", len(results), mismatches)

	// -------------------------------------------------------------------------
	// Update catalog files if requested.

	if !update || mismatches == 0 {
		return nil
	}

	fmt.Println()
	fmt.Println("Updating catalog files...")

	type correction struct {
		arch     string
		ggufArch string
	}

	corrections := make(map[string]correction)
	for _, r := range results {
		if !r.downloaded {
			continue
		}

		c := correction{ggufArch: r.ggufArch}
		if !r.match && r.detectedType != "" {
			c.arch = r.detectedType
		}
		corrections[r.modelID] = c
	}

	files, err := os.ReadDir(catalogRepoPath)
	if err != nil {
		return fmt.Errorf("reading catalog directory: %w", err)
	}

	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".yaml" {
			continue
		}

		filePath := filepath.Join(catalogRepoPath, f.Name())

		data, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("reading catalog file %s: %w", f.Name(), err)
		}

		var cat catalog.CatalogModels
		if err := yaml.Unmarshal(data, &cat); err != nil {
			return fmt.Errorf("unmarshaling catalog file %s: %w", f.Name(), err)
		}

		updated := false
		for i, m := range cat.Models {
			c, exists := corrections[m.ID]
			if !exists {
				continue
			}

			if c.arch != "" {
				fmt.Printf("  %s: %s -> %s (%s)\n", f.Name(), m.Architecture, c.arch, m.ID)
				cat.Models[i].Architecture = c.arch
			}

			if c.ggufArch != "" && cat.Models[i].GGUFArch != c.ggufArch {
				fmt.Printf("  %s: gguf_arch -> %s (%s)\n", f.Name(), c.ggufArch, m.ID)
				cat.Models[i].GGUFArch = c.ggufArch
			}

			updated = true
		}

		if !updated {
			continue
		}

		out, err := yaml.Marshal(&cat)
		if err != nil {
			return fmt.Errorf("marshaling catalog file %s: %w", f.Name(), err)
		}

		out = addBlankLinesBetweenModels(out)

		if err := os.WriteFile(filePath, out, 0644); err != nil {
			return fmt.Errorf("writing catalog file %s: %w", f.Name(), err)
		}
	}

	fmt.Println("Done.")

	return nil
}

// modelTypeToArch converts a model.ModelType to the catalog architecture
// string (Dense, MoE, Hybrid).
func modelTypeToArch(mt model.ModelType) string {
	switch mt {
	case model.ModelTypeMoE:
		return "MoE"
	case model.ModelTypeHybrid:
		return "Hybrid"
	default:
		return "Dense"
	}
}

// addBlankLinesBetweenModels inserts a blank line before each model entry
// for readability, matching the existing catalog file format.
func addBlankLinesBetweenModels(data []byte) []byte {
	return []byte(strings.ReplaceAll(string(data), "\n- id:", "\n\n- id:"))
}
