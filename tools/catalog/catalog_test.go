package catalog_test

import (
	"context"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/tools/catalog"
	"github.com/google/go-cmp/cmp"
)

func Test_Catalog(t *testing.T) {
	ctx := context.Background()
	basePath := t.TempDir()

	if err := catalog.Download(ctx, basePath); err != nil {
		t.Fatalf("download catalog: %v", err)
	}

	catalogs, err := catalog.RetrieveCatalogs(basePath)
	if err != nil {
		t.Fatalf("retrieve catalog: %v", err)
	}

	expCat := catalog.Catalog{
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

	var gotCat catalog.Catalog
	for _, cat := range catalogs {
		if cat.Models[0].ID == expCat.Models[0].ID {
			gotCat = cat
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
