package models

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func blockDraftTestNetwork(t *testing.T) {
	t.Helper()
	previous := http.DefaultTransport
	transport := &http.Transport{DialContext: func(_ context.Context, _, address string) (net.Conn, error) {
		t.Errorf("unexpected real network connection to %s", address)
		return nil, errors.New("test network blocked")
	}}
	http.DefaultTransport = transport
	t.Cleanup(func() { transport.CloseIdleConnections(); http.DefaultTransport = previous })
}

func TestGenericDraftDownloadAndRestore(t *testing.T) {
	blockDraftTestNetwork(t)
	t.Setenv("KRONK_SKIP_NETWORK_CHECK", "true")
	for _, method := range []DraftMethod{DraftMethodDSpark, DraftMethodConventional, DraftMethodMTP} {
		t.Run(method.String(), func(t *testing.T) {
			modelFile := filepath.Join(t.TempDir(), "model.gguf")
			writeCatalogTestGGUF(t, modelFile, nil)
			body, err := os.ReadFile(modelFile)
			if err != nil {
				t.Fatal(err)
			}
			g := &fakeGetter{contents: map[string][]byte{
				"/target/repo/resolve/main/model-Q4.gguf":                body,
				"/another/drafts/resolve/revision/nested/head.gguf_file": []byte("tiny draft"),
				"/target/repo/resolve/main/mmproj.gguf":                  []byte("tiny projection"),
			}}
			withFakeGetter(t, g)
			m := newTestModels(t)
			draft := CatalogDraft{Method: method, File: "custom-installed.gguf", Original: "nested/head.gguf_file",
				URL: "https://huggingface.co/another/drafts/resolve/revision/nested/head.gguf_file", Size: 10}
			r := NewResolver(m, filepath.Join(m.BasePath(), "catalog", "catalog.yaml"))
			entry := CatalogEntry{Provider: "target", Family: "repo", Revision: "main", Files: []string{"model-Q4.gguf"}, Draft: draft, DraftChecked: true}
			if err := r.Save(Catalog{Version: catalogVersion, Models: map[string]CatalogEntry{"target/model-Q4": entry}}); err != nil {
				t.Fatal(err)
			}
			mp, err := m.Download(t.Context(), testLog, "target/model-Q4")
			if err != nil {
				t.Fatal(err)
			}
			want := filepath.Join(m.Path(), "target", "repo", draft.File)
			if mp.DraftFile != want || (mp.MTPFile != "") != (method == DraftMethodMTP) {
				t.Fatalf("installed draft: got %+v, want %q with MTP only for mtp", mp, want)
			}
			indexed, err := m.FullPath("target/model-Q4")
			if err != nil || indexed.DraftFile != want || indexed.MTPFile != mp.MTPFile {
				t.Fatalf("index: got %+v, %v", indexed, err)
			}
			files, err := m.Files()
			if err != nil || len(files) != 1 {
				t.Fatalf("draft indexed as standalone model: got %+v, %v", files, err)
			}
			inventory, err := m.IntegrityFor("target/model-Q4")
			if err != nil || !inventory.Verified || len(inventory.Artifacts) != 2 {
				t.Fatalf("integrity: got %+v, %v", inventory, err)
			}
			res, err := m.ResolveSource(t.Context(), "target/model-Q4")
			if err != nil || res.LocalDraft != want || res.DownloadDraft != draft.URL || res.VerifyLocal() != nil {
				t.Fatalf("resolution: got %+v, %v", res, err)
			}
			if err := os.Remove(want); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(artifactDigestPath(want)); err != nil {
				t.Fatal(err)
			}
			if err := m.BuildIndex(testLog, true); err != nil {
				t.Fatal(err)
			}
			missing, err := m.FullPath("target/model-Q4")
			if err != nil || missing.Validated {
				t.Fatalf("missing draft remained validated: got %+v, %v", missing, err)
			}
			previousNetwork := hasNetworkFn
			hasNetworkFn = func() bool { return false }
			_, offlineErr := m.Download(t.Context(), testLog, "target/model-Q4")
			hasNetworkFn = previousNetwork
			if offlineErr == nil {
				t.Fatal("offline download accepted a missing draft")
			}
			if _, err := m.Download(t.Context(), testLog, "target/model-Q4"); err != nil {
				t.Fatalf("restore missing companion: %v", err)
			}
			if err := checkModelStrict(t.Context(), testLog, want); err != nil {
				t.Fatal(err)
			}
			// The projection override uses the public generic API and must not
			// reinterpret a DSpark/conventional companion as MTP.
			mp, err = m.DownloadDraftURLsWithProgress(t.Context(), testLog,
				[]string{"https://huggingface.co/target/repo/resolve/main/model-Q4.gguf"},
				"https://huggingface.co/target/repo/resolve/main/mmproj.gguf", draft, nil)
			if err != nil || mp.DraftFile != want || (mp.MTPFile != "") != (method == DraftMethodMTP) {
				t.Fatalf("projection override: got %+v, %v", mp, err)
			}
			cat, err := r.Load()
			if err != nil || cat.Models["target/model-Q4"].Draft != draft {
				t.Fatalf("metadata refresh changed draft: got %+v, %v", cat.Models["target/model-Q4"].Draft, err)
			}
			if err := m.Remove(mp, testLog); err != nil {
				t.Fatal(err)
			}
			for _, path := range []string{want, artifactDigestPath(want), artifactVerificationPath(want)} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Errorf("Remove left %q: %v", path, err)
				}
			}
		})
	}
}

func TestGenericDraftSummary(t *testing.T) {
	for _, method := range []DraftMethod{{}, DraftMethodMTP, DraftMethodDSpark, DraftMethodConventional} {
		t.Run(method.String(), func(t *testing.T) {
			entry := CatalogEntry{Provider: "target", Family: "repo", Files: []string{"model.gguf"}, FileSizes: []int64{100}}
			if !method.IsZero() {
				entry.Draft = CatalogDraft{Method: method, File: "head.gguf", URL: "https://huggingface.co/other/repo/resolve/main/head.gguf_file", Size: 20}
			}
			summary := NewSummary("target/model", entry, nil, nil)
			files := NewFiles(entry)
			if summary.HasDraft != !method.IsZero() || summary.HasMTP != (method == DraftMethodMTP) || summary.Draft != entry.Draft ||
				summary.Capabilities.Draft != method || summary.Capabilities.MTP != (method == DraftMethodMTP) ||
				summary.TotalSizeBytes != 100+entry.Draft.Size || files.Draft.URL != entry.Draft.URL || (files.MTP.URL != "") != (method == DraftMethodMTP) {
				t.Fatalf("summary/files: got %+v, %+v", summary, files)
			}
			data, err := json.Marshal(summary)
			if err != nil || strings.Contains(string(data), `"draft":`) != !method.IsZero() {
				t.Fatalf("draft omission: got %s, %v", data, err)
			}
		})
	}
}

func TestPinnedRefreshPreservesGenericDraft(t *testing.T) {
	file := filepath.Join(t.TempDir(), "catalog.yaml")
	draft := CatalogDraft{Method: DraftMethodDSpark, File: "custom.gguf", Original: "nested/head.gguf_file", URL: "https://huggingface.co/other/drafts/resolve/main/nested/head.gguf_file", Size: 20}
	r := newTestResolver(nil, file, &fakeHF{metas: map[string][]string{"target/repo": {"model-Q4.gguf", "mtp-model.gguf"}}})
	if err := r.Save(Catalog{Version: catalogVersion, Models: map[string]CatalogEntry{"target/model-Q4": {
		Provider: "target", Family: "repo", Revision: "main", Files: []string{"model-Q4.gguf"}, Draft: draft, DraftChecked: false,
	}}}); err != nil {
		t.Fatal(err)
	}
	res, err := r.resolvePinned(t.Context(), "target", "repo", "model-Q4", true)
	if err != nil || res.Draft != draft || res.DraftChecked || res.DownloadDraft != draft.URL {
		t.Fatalf("fresh resolution: got %+v, %v", res, err)
	}
	cat, err := r.Load()
	if err != nil || cat.Models["target/model-Q4"].Draft != draft || cat.Models["target/model-Q4"].DraftChecked {
		t.Fatalf("fresh persisted entry: got %+v, %v", cat, err)
	}
}

func TestDraftMTPCompatibilityMetadata(t *testing.T) {
	dspark := CatalogEntry{
		Draft:     CatalogDraft{Method: DraftMethodDSpark, File: "draft.gguf"},
		MTPSource: MTPSourceCompanion, Capabilities: CatalogCapabilities{MTP: true},
	}
	summary := NewSummary("owner/model", dspark, nil, nil)
	if summary.HasMTP || !summary.MTPSource.IsZero() || summary.Capabilities.MTP {
		t.Fatalf("DSpark advertised as MTP: %+v", summary)
	}
	embedded := CatalogEntry{MTPSource: MTPSourceEmbedded, Capabilities: CatalogCapabilities{MTP: true}}
	summary = NewSummary("owner/embedded", embedded, nil, nil)
	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if !summary.Capabilities.MTP || summary.Capabilities.Draft != DraftMethodMTP || summary.MTPSource != MTPSourceEmbedded || summary.HasDraft || fields["draft"] != nil {
		t.Fatalf("embedded MTP advertised as a companion: %+v, %s, %v", summary, data, err)
	}
}

func TestURLOnlyMTPOverride(t *testing.T) {
	blockDraftTestNetwork(t)
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{false: "generic", true: "legacy"}[legacy], func(t *testing.T) {
			g := &fakeGetter{contents: map[string][]byte{
				"/target/repo/resolve/main/model-Q4.gguf":       []byte("tiny model"),
				"/other/drafts/resolve/pinned/nested/head.gguf": []byte("tiny head"),
			}}
			withFakeGetter(t, g)
			m := newTestModels(t)
			urls := []string{"https://huggingface.co/target/repo/resolve/main/model-Q4.gguf"}
			const draftURL = "https://huggingface.co/other/drafts/resolve/pinned/nested/head.gguf"
			var mp Path
			var err error
			switch legacy {
			case true:
				mp, err = m.DownloadURLsWithProgress(t.Context(), testLog, urls, "", draftURL, nil)
			default:
				mp, err = m.DownloadDraftURLsWithProgress(t.Context(), testLog, urls, "", CatalogDraft{Method: DraftMethodMTP, URL: draftURL}, nil)
			}
			if err != nil || filepath.Base(mp.MTPFile) != "mtp-model-Q4.gguf" || mp.DraftFile != mp.MTPFile {
				t.Fatalf("MTP override: got %+v, %v", mp, err)
			}
			cat, err := m.Catalog()
			if err != nil {
				t.Fatal(err)
			}
			draft := cat.Models["target/model-Q4"].Draft
			if draft.Method != DraftMethodMTP || draft.File != "mtp-model-Q4.gguf" || draft.Original != "nested/head.gguf" || draft.URL != draftURL {
				t.Fatalf("MTP override persisted: got %+v", draft)
			}
		})
	}
}
