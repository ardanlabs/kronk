package catalog_test

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/tools/catalog"
	"github.com/google/go-cmp/cmp"
)

//go:embed test_data/*
var testData embed.FS

var cat *catalog.Catalog

func TestMain(m *testing.M) {
	basePath, err := os.MkdirTemp("", "catalog-test-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "create temp dir: %v\n", err)
		os.Exit(1)
	}

	if err := setupTestCatalog(basePath); err != nil {
		os.RemoveAll(basePath)
		fmt.Fprintf(os.Stderr, "setup test catalog: %v\n", err)
		os.Exit(1)
	}

	cat, err = catalog.NewWithSettings(basePath, "")
	if err != nil {
		os.RemoveAll(basePath)
		fmt.Fprintf(os.Stderr, "setup test catalog: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()
	defer func() {
		recover()
		os.RemoveAll(basePath)
	}()

	os.Exit(code)
}

func Test_Catalog(t *testing.T) {
	catalogs, err := cat.RetrieveCatalogs()
	if err != nil {
		t.Fatalf("retrieve catalog: %v", err)
	}

	expCat := catalog.CatalogModels{
		Name: "Text-Generation",
		Models: []catalog.Model{
			{
				ID:          "Qwen3-8B-Q8_0",
				Category:    "Text-Generation",
				OwnedBy:     "Qwen",
				ModelFamily: "Qwen3-8B-GGUF",
				WebPage:     "https://huggingface.co/Qwen/Qwen3-8B-GGUF",
				Files: catalog.Files{
					Model: catalog.File{
						URL:  "https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf",
						Size: "8.71 GiB",
					},
				},
				Capabilities: catalog.Capabilities{
					Endpoint:  "chat_completion",
					Images:    false,
					Audio:     false,
					Video:     false,
					Streaming: true,
					Reasoning: true,
					Tooling:   true,
				},
				Metadata: catalog.Metadata{
					Created:     time.Date(2025, 5, 3, 0, 0, 0, 0, time.UTC),
					Collections: "https://huggingface.co/collections/Qwen",
					Description: "Qwen3 is the latest generation of large language models in Qwen series, offering a comprehensive suite of dense and mixture-of-experts (MoE) models.",
				},
			},
		},
	}

	if len(catalogs) == 0 {
		t.Fatal("no catalogs returned")
	}

	var gotCat catalog.CatalogModels
	for _, catalog := range catalogs {
		if len(catalog.Models) == 0 {
			continue
		}

		if catalog.Models[0].ID == expCat.Models[0].ID {
			gotCat = catalog
			break
		}
	}

	if len(gotCat.Models) > 1 {
		gotCat.Models = gotCat.Models[:1]
	}

	if diff := cmp.Diff(expCat, gotCat); diff != "" {
		t.Errorf("catalog mismatch (-want +got):\n%s", diff)
		t.Log("============================================")
		t.Logf("got: %#v\n", gotCat)
		t.Log("============================================")
		t.Logf("exp: %#v\n", expCat)
	}
}

func Test_RetrieveCatalogs(t *testing.T) {
	catalogs, err := cat.RetrieveCatalogs()
	if err != nil {
		t.Fatalf("retrieve catalogs: %v", err)
	}

	if len(catalogs) != 4 {
		t.Errorf("expected 4 catalogs, got %d", len(catalogs))
	}

	catalogNames := make(map[string]bool)
	for _, cat := range catalogs {
		catalogNames[cat.Name] = true
	}

	expectedNames := []string{"Text-Generation", "Embedding", "Audio-Text-to-Text", "Image-Text-to-Text"}
	for _, name := range expectedNames {
		if !catalogNames[name] {
			t.Errorf("expected catalog %q not found", name)
		}
	}
}

func Test_RetrieveCatalog(t *testing.T) {
	catalog, err := cat.RetrieveCatalog("text_generation.yaml")
	if err != nil {
		t.Fatalf("retrieve catalog: %v", err)
	}

	if catalog.Name != "Text-Generation" {
		t.Errorf("expected catalog name %q, got %q", "Text-Generation", catalog.Name)
	}

	if len(catalog.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(catalog.Models))
	}

	modelIDs := make(map[string]bool)
	for _, model := range catalog.Models {
		modelIDs[model.ID] = true
	}

	if !modelIDs["Qwen3-8B-Q8_0"] {
		t.Error("expected model Qwen3-8B-Q8_0 not found")
	}

	if !modelIDs["gpt-oss-20b-Q8_0"] {
		t.Error("expected model gpt-oss-20b-Q8_0 not found")
	}
}

func Test_RetrieveModelDetails(t *testing.T) {
	model, err := cat.RetrieveModelDetails("qwen3-8b-q8_0")
	if err != nil {
		t.Fatalf("retrieve model details: %v", err)
	}

	if model.ID != "Qwen3-8B-Q8_0" {
		t.Errorf("expected model ID %q, got %q", "Qwen3-8B-Q8_0", model.ID)
	}

	if model.Category != "Text-Generation" {
		t.Errorf("expected category %q, got %q", "Text-Generation", model.Category)
	}

	if model.OwnedBy != "Qwen" {
		t.Errorf("expected owned_by %q, got %q", "Qwen", model.OwnedBy)
	}

	if model.Capabilities.Endpoint != "chat_completion" {
		t.Errorf("expected endpoint %q, got %q", "chat_completion", model.Capabilities.Endpoint)
	}
}

// =============================================================================

func setupTestCatalog(basePath string) error {
	catalogDir := filepath.Join(basePath, "catalogs")

	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		return fmt.Errorf("create catalog dir: %w", err)
	}

	entries, err := testData.ReadDir("test_data")
	if err != nil {
		return fmt.Errorf("read test_data dir: %w", err)
	}

	for _, entry := range entries {
		data, err := testData.ReadFile("test_data/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read test file %s: %w", entry.Name(), err)
		}

		if err := os.WriteFile(filepath.Join(catalogDir, entry.Name()), data, 0644); err != nil {
			return fmt.Errorf("write test file %s: %w", entry.Name(), err)
		}
	}

	return nil
}
