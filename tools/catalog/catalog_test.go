package catalog_test

import (
	"context"
	"testing"

	"github.com/ardanlabs/kronk/tools/catalog"
)

func Test_Catalog(t *testing.T) {
	ctx := context.Background()
	basePath := t.TempDir()

	if err := catalog.Download(ctx, basePath); err != nil {
		t.Fatalf("download catalog: %v", err)
	}

	catalogs, err := catalog.Retrieve(basePath)
	if err != nil {
		t.Fatalf("retrieve catalog: %v", err)
	}

	var textGen catalog.Catalog
	for _, c := range catalogs {
		if c.Name == "Text-Generation" {
			textGen = c
			break
		}
	}

	if textGen.Name == "" {
		t.Fatal("Text-Generation catalog not found")
	}

	if len(textGen.Models) == 0 {
		t.Fatal("Text-Generation catalog has no models")
	}

	model := textGen.Models[0]

	if model.ID != "Qwen3-8B-Q8_0" {
		t.Errorf("expected model ID Qwen3-8B-Q8_0, got %s", model.ID)
	}

	if model.Category != "Text-Generation" {
		t.Errorf("expected category Text-Generation, got %s", model.Category)
	}

	if model.OwnedBy != "Qwen" {
		t.Errorf("expected owned_by Qwen, got %s", model.OwnedBy)
	}

	if model.Files.Model.URL == "" {
		t.Error("expected model file URL to be set")
	}

	if model.Capabilities.Endpoint != "chat_completion" {
		t.Errorf("expected endpoint chat_completion, got %s", model.Capabilities.Endpoint)
	}

	if !model.Capabilities.Streaming {
		t.Error("expected streaming capability to be true")
	}

	if !model.Capabilities.Tooling {
		t.Error("expected tooling capability to be true")
	}
}
