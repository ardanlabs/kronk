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

	cat, err = catalog.New(catalog.WithBasePath(basePath))
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
				ID:          "Llama-3.3-70B-Instruct-Q8_0",
				Category:    "Text-Generation",
				OwnedBy:     "unsloth",
				ModelFamily: "Llama-3.3-70B-Instruct-GGUF",
				WebPage:     "https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF",
				Files: catalog.Files{
					Models: []catalog.File{
						{
							URL:  "https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/resolve/main/Llama-3.3-70B-Instruct-Q8_0/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf",
							Size: "39.8 GiB",
						},
						{
							URL:  "https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/resolve/main/Llama-3.3-70B-Instruct-Q8_0/Llama-3.3-70B-Instruct-Q8_0-00002-of-00002.gguf",
							Size: "35.2 GiB",
						},
					},
					Proj: catalog.File{
						URL:  "proj: just for testing",
						Size: "0.0 GiB",
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
					Created:     time.Date(2025, 5, 10, 0, 0, 0, 0, time.UTC),
					Collections: "https://huggingface.co/collections/unsloth",
					Description: "Llama 3.3 70B is Meta's advanced, multilingual, open-source large language model (LLM) with 70 billion parameters, excelling in complex reasoning, dialogue, and coding tasks, delivering flagship-level performance (like 405B models) with better efficiency, optimized for text-only applications, and featuring improved instruction-following, safety, and tool-use capabilities for enterprise and research use.",
				},
			},
		},
	}

	if len(catalogs) == 0 {
		t.Fatal("no catalogs returned")
	}

	var gotCat catalog.CatalogModels
outer:
	for _, cat := range catalogs {
		for _, model := range cat.Models {
			if model.ID == expCat.Models[0].ID {
				gotCat = cat
				break outer
			}
		}
	}

	for i, model := range gotCat.Models {
		if model.ID == expCat.Models[0].ID {
			gotCat.Models = []catalog.Model{gotCat.Models[i]}
			break
		}
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

	if len(catalog.Models) != 3 {
		t.Errorf("expected 3 models, got %d", len(catalog.Models))
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
