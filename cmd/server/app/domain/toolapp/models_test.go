package toolapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestSSEPayloadsEndWithBlankLine(t *testing.T) {
	events := []string{
		toAppVersion("downloading", libs.VersionTag{}, false),
		toAppPull("downloading", models.Path{}),
		toAppPullResponse(PullResponse{Status: "downloading"}),
	}

	for i, event := range events {
		if !strings.HasSuffix(event, "\n\n") {
			t.Errorf("event %d: got %q, want blank-line terminator", i, event)
		}
	}
}

func TestCatalogDraftPreview(t *testing.T) {
	t.Setenv("KRONK_SKIP_NETWORK_CHECK", "true")
	for _, tt := range []struct {
		name   string
		method models.DraftMethod
	}{
		{"MTP", models.DraftMethodMTP},
		{"DSpark", models.DraftMethodDSpark},
		{"conventional", models.DraftMethodConventional},
	} {
		t.Run(tt.name, func(t *testing.T) {
			base := t.TempDir()
			mdls, err := models.NewWithPaths(base)
			if err != nil {
				t.Fatal(err)
			}
			const id = "target/model-Q8_0"
			const draftURL = "https://huggingface.co/other/drafters/resolve/main/nested/draft.gguf_file"
			entry := models.CatalogEntry{
				Provider: "target", Family: "model-GGUF", Revision: "main",
				Files: []string{"model-Q8_0.gguf"}, DraftChecked: true,
				Draft: models.CatalogDraft{Method: tt.method, File: "draft-model-Q8_0.gguf", URL: draftURL, Size: 73},
			}
			resolver := models.NewResolver(mdls, filepath.Join(base, "catalog", "catalog.yaml"))
			if err := resolver.Save(models.Catalog{Version: "3.1", Models: map[string]models.CatalogEntry{id: entry}}); err != nil {
				t.Fatal(err)
			}
			a := app{models: mdls}
			request := httptest.NewRequest(http.MethodPost, "/v1/kronk/catalog/resolve", strings.NewReader(`{"source":"`+id+`"}`))
			request.Header.Set("Content-Type", "application/json")
			data, _, err := a.resolveCatalog(t.Context(), request).Encode()
			if err != nil {
				t.Fatal(err)
			}
			var preview ResolveResponse
			if err := json.Unmarshal(data, &preview); err != nil {
				t.Fatal(err)
			}
			if preview.Draft.Method != tt.method || preview.Draft.File != "draft-model-Q8_0.gguf" || preview.DownloadDraft != draftURL || preview.Installed {
				t.Fatalf("preview: got %+v, want selected draft and not installed", preview)
			}
			wantMTPURL := ""
			if tt.method == models.DraftMethodMTP {
				wantMTPURL = draftURL
			}
			if preview.DownloadMTP != wantMTPURL {
				t.Errorf("MTP compatibility URL: got %q, want %q", preview.DownloadMTP, wantMTPURL)
			}
			data, _, err = a.listCatalog(t.Context(), httptest.NewRequest(http.MethodGet, "/v1/kronk/catalog", nil)).Encode()
			if err != nil {
				t.Fatal(err)
			}
			var list []models.CatalogSummary
			if err := json.Unmarshal(data, &list); err != nil {
				t.Fatal(err)
			}
			if len(list) != 1 || !list[0].HasDraft || list[0].Draft.Method != tt.method || list[0].Capabilities.Draft != tt.method || list[0].HasMTP != (tt.method == models.DraftMethodMTP) {
				t.Errorf("list: got %+v, want generic draft with MTP only for MTP method", list)
			}
		})
	}
}

func TestToPullProgress(t *testing.T) {
	progress := models.DownloadProgress{
		Src:          "model.gguf",
		CurrentBytes: 1_234_567,
		TotalBytes:   9_876_543,
		MBPerSec:     12.34,
		Complete:     true,
	}

	got := toPullProgress(progress)

	wantStatus := "download-model: Downloading model.gguf... 1 MB of 9 MB (12.34 MB/s)"
	if got.Status != wantStatus {
		t.Errorf("Status: got %q, want %q", got.Status, wantStatus)
	}
	if got.Progress == nil {
		t.Fatal("Progress: got nil, want structured progress")
	}
	if got.Progress.Src != progress.Src {
		t.Errorf("Src: got %q, want %q", got.Progress.Src, progress.Src)
	}
	if got.Progress.CurrentBytes != progress.CurrentBytes {
		t.Errorf("CurrentBytes: got %d, want %d", got.Progress.CurrentBytes, progress.CurrentBytes)
	}
	if got.Progress.TotalBytes != progress.TotalBytes {
		t.Errorf("TotalBytes: got %d, want %d", got.Progress.TotalBytes, progress.TotalBytes)
	}
	if got.Progress.MBPerSec != progress.MBPerSec {
		t.Errorf("MBPerSec: got %f, want %f", got.Progress.MBPerSec, progress.MBPerSec)
	}
	if got.Progress.Complete != progress.Complete {
		t.Errorf("Complete: got %t, want %t", got.Progress.Complete, progress.Complete)
	}
}
