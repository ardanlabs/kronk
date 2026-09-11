package toolapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
)

func TestListMalinaCatalog(t *testing.T) {
	resp := (&app{}).listMalinaCatalog(t.Context(), httptest.NewRequest(http.MethodGet, "/malina/models/catalog", nil))
	catalog, ok := resp.(MalinaCatalogResponse)
	if !ok {
		t.Fatalf("listMalinaCatalog() response = %T, want MalinaCatalogResponse", resp)
	}

	if len(catalog.Models) != 8 {
		t.Fatalf("models: got %d, want 8", len(catalog.Models))
	}

	entries := make(map[string]MalinaCatalogEntry, len(catalog.Models))
	for _, entry := range catalog.Models {
		entries[entry.ID] = entry
	}

	flux := entries["flux2-klein-9b"]
	if !flux.Gated || !flux.BasicTextToImage || len(flux.Files) != 3 {
		t.Errorf("flux2-klein-9b: got %+v, want gated text-to-image bundle with 3 files", flux)
	}

	upscaler := entries["realesrgan-x4-anime"]
	if upscaler.BasicTextToImage || len(upscaler.Files) != 1 || upscaler.Files[0].Role != "upscaler" {
		t.Errorf("realesrgan-x4-anime: got %+v, want one-file support bundle", upscaler)
	}
}

func TestListMalinaModelsIncludesSupportBundles(t *testing.T) {
	basePath := t.TempDir()
	models, err := malinamodels.NewWithPaths(basePath)
	if err != nil {
		t.Fatalf("NewWithPaths() error = %v", err)
	}

	bundle, ok := malinamodels.BundleByName(malinamodels.BundleRealESRGANX4Anime)
	if !ok {
		t.Fatal("BundleByName() did not find realesrgan-x4-anime")
	}

	bundlePath := filepath.Join(models.Path(), bundle.Name.String())
	if err := os.MkdirAll(bundlePath, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}

	modelPath := filepath.Join(bundlePath, bundle.Files[0].Filename)
	if err := os.WriteFile(modelPath, []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile(model) error = %v", err)
	}

	manifest := malinamodels.Manifest{
		Bundle:  bundle.Name,
		License: bundle.License,
		Gated:   bundle.Gated,
		Files:   map[string]string{string(bundle.Files[0].Role): modelPath},
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(bundlePath, malinamodels.ManifestFilename), data, 0o644); err != nil {
		t.Fatalf("WriteFile(manifest) error = %v", err)
	}

	resp := (&app{malinaModels: models}).listMalinaModels(t.Context(), httptest.NewRequest(http.MethodGet, "/malina/models", nil))
	list, ok := resp.(MalinaModelsResponse)
	if !ok {
		t.Fatalf("listMalinaModels() response = %T, want MalinaModelsResponse", resp)
	}
	if len(list.Models) != 1 {
		t.Fatalf("models: got %d, want 1", len(list.Models))
	}
	if list.Models[0].ID != bundle.Name.String() || list.Models[0].BasicTextToImage {
		t.Errorf("model: got %+v, want installed support bundle", list.Models[0])
	}
}
