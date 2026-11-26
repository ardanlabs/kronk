package toolapp

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
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
			if err := resolver.Save(models.Catalog{Version: "3.2", Models: map[string]models.CatalogEntry{id: entry}}); err != nil {
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

func TestPullDraftCompanion(t *testing.T) {
	t.Setenv("KRONK_SKIP_NETWORK_CHECK", "true")
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprintf("override=%t", override), func(t *testing.T) {
			const modelFile = "model-Q4.gguf"
			const original = "nested/dspark-head-Q8.gguf_file"
			bodies := map[string]string{
				modelFile: "tiny target fixture", "mmproj.gguf": "tiny projection fixture", original: "different draft fixture",
			}
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				parts := strings.SplitN(r.URL.Path, "/main/", 2)
				if len(parts) != 2 {
					http.NotFound(w, r)
					return
				}
				body, ok := bodies[parts[1]]
				if !ok {
					t.Errorf("unexpected fixture request: %s", r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if strings.Contains(r.URL.Path, "/raw/") {
					body = fmt.Sprintf("version https://git-lfs.github.com/spec/v1\noid sha256:%x\nsize %d\n", sha256.Sum256([]byte(body)), len(body))
				}
				http.ServeContent(w, r, filepath.Base(parts[1]), time.Time{}, strings.NewReader(body))
			}))
			defer peer.Close()
			base := t.TempDir()
			mdls, err := models.NewWithPaths(base)
			if err != nil {
				t.Fatal(err)
			}
			const id = "target/model-Q4"
			draft := models.CatalogDraft{
				Method: models.DraftMethodDSpark, File: "dspark-model-Q4.gguf", Original: original,
				URL: "https://huggingface.co/other/drafters/resolve/main/" + original,
			}
			entry := models.CatalogEntry{
				Provider: "target", Family: "repo", Revision: "main", Files: []string{modelFile},
				MMProj: "mmproj.gguf", MMProjOrig: "mmproj.gguf", DraftChecked: true, Draft: draft,
			}
			req := PullRequest{ModelURL: id, DownloadServer: strings.TrimPrefix(peer.URL, "http://")}
			if override {
				entry.Draft = models.CatalogDraft{Method: models.DraftMethodMTP, File: "mtp-old.gguf", Original: "old.gguf"}
				req.Draft = models.CatalogDraft{Method: draft.Method, URL: draft.URL}
			}
			r := models.NewResolver(mdls, filepath.Join(base, "catalog", "catalog.yaml"))
			if err := r.Save(models.Catalog{Version: "3.2", Models: map[string]models.CatalogEntry{id: entry}}); err != nil {
				t.Fatal(err)
			}
			log := logger.New(io.Discard, logger.LevelInfo, "test", nil)
			a := app{models: mdls, log: log}
			api := web.NewApp(func(context.Context, string, ...any) {})
			api.HandlerFunc(http.MethodPost, "v1", "/kronk/models/pull", a.pullModels)
			data, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			request := httptest.NewRequest(http.MethodPost, "/v1/kronk/models/pull", strings.NewReader(string(data)))
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, request)
			var complete PullResponse
			var meta *PullMeta
			for line := range strings.SplitSeq(rec.Body.String(), "\n") {
				if payload, ok := strings.CutPrefix(line, "data: "); ok {
					var event PullResponse
					if err := json.Unmarshal([]byte(payload), &event); err != nil {
						t.Fatal(err)
					}
					complete = event
					if event.Meta != nil {
						meta = event.Meta
					}
				}
			}
			if complete.Status != "downloaded" || !complete.Downloaded || meta == nil || meta.Draft.Method != draft.Method || !strings.Contains(meta.Draft.URL, original) {
				t.Fatalf("pull result: %+v, meta %+v; stream:\n%s", complete, meta, rec.Body.String())
			}
			if filepath.Base(complete.DraftFile) != draft.File || complete.ProjFile == "" {
				t.Fatalf("companions: got %+v, want DSpark and preserved projection", complete)
			}
			body, err := os.ReadFile(complete.DraftFile)
			if err != nil || string(body) != bodies[original] {
				t.Fatalf("installed draft: got %q, %v", body, err)
			}
			installed, err := mdls.FullPath(id)
			if err != nil || installed.DraftFile != complete.DraftFile || installed.MTPFile != "" {
				t.Fatalf("indexed draft: got %+v, %v, want DSpark file %q", installed, err, complete.DraftFile)
			}
			catalog, err := mdls.Catalog()
			if err != nil || catalog.Models[id].Draft.URL != draft.URL {
				t.Fatalf("catalog draft source: got %+v, %v, want upstream %s", catalog.Models[id].Draft, err, draft.URL)
			}
		})
	}
}

func TestPullDraftValidation(t *testing.T) {
	for _, payload := range []string{
		`{"draft":{"url":"https://huggingface.co/other/repo/resolve/main/head.gguf"}}`,
		`{"draft":{"method":"dspark"}}`,
		`{"draft":{"method":"unknown","url":"https://huggingface.co/other/repo/resolve/main/head.gguf"}}`,
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/kronk/models/pull", strings.NewReader(payload))
		var pull PullRequest
		if err := web.Decode(req, &pull); err == nil {
			t.Errorf("invalid draft request accepted: %s", payload)
		}
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
