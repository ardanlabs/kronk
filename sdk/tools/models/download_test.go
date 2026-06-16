package models

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
	"github.com/ardanlabs/kronk/sdk/tools/downloader"
)

// =============================================================================
// fake getter — hermetic stand-in for downloader.Download. Mirrors the
// fakeHF pattern from catalog_test.go: in-memory, records every call,
// returns content from a pre-populated map.

// fakeGetter is a hermetic replacement for downloader.Download. It writes
// matching content to dest, generating sha pointer files dynamically from
// the underlying model bytes when the URL is a "/raw/" URL. The fake honors
// the ?filename= query parameter that withDestFilename appends.
type fakeGetter struct {
	// contents maps the URL path (e.g. "/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf")
	// to the raw body bytes for the file at that resolve URL. The sha
	// pointer for the matching /raw/ URL is computed on the fly.
	contents map[string][]byte

	// failOnURLSubstr maps a URL substring to an error. The first match
	// (deterministic by Go map iteration is OK here — tests register at
	// most one fail rule) wins, allowing tests to simulate partial /
	// network failures on specific shards.
	failOnURLSubstr map[string]error

	// truncateBytes maps a URL substring to a byte count; when the URL
	// matches, only the first N bytes of the body are written. Used to
	// simulate an interrupted download for resume tests.
	truncateBytes map[string]int

	mu    sync.Mutex
	calls []string
}

func (f *fakeGetter) download(_ context.Context, src string, dest string, _ downloader.ProgressFunc, _ int64) (bool, error) {
	f.mu.Lock()
	f.calls = append(f.calls, src)
	f.mu.Unlock()

	for sub, err := range f.failOnURLSubstr {
		if strings.Contains(src, sub) {
			return false, err
		}
	}

	u, err := url.Parse(src)
	if err != nil {
		return false, fmt.Errorf("fake-getter: parse %q: %w", src, err)
	}

	// Destination filename — ?filename= query param wins over URL basename.
	name := u.Query().Get("filename")
	if name == "" {
		name = path.Base(u.Path)
	}

	// /raw/ URLs return a generated sha pointer for the matching /resolve/
	// URL's body. /resolve/ URLs return the body directly.
	var body []byte
	switch {
	case strings.Contains(u.Path, "/raw/"):
		resolveKey := strings.Replace(u.Path, "/raw/", "/resolve/", 1)
		modelBody, ok := f.contents[resolveKey]
		if !ok {
			return false, fmt.Errorf("fake-getter: no content registered for %s", resolveKey)
		}
		body = makeShaPointer(modelBody)

	case strings.Contains(u.Path, "/resolve/"):
		modelBody, ok := f.contents[u.Path]
		if !ok {
			return false, fmt.Errorf("fake-getter: no content registered for %s", u.Path)
		}
		body = modelBody

	default:
		return false, fmt.Errorf("fake-getter: unrecognized URL shape %q", u.Path)
	}

	if n, ok := f.truncateBytes[src]; ok && n < len(body) {
		body = body[:n]
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return false, fmt.Errorf("fake-getter: mkdir %s: %w", dest, err)
	}

	if err := os.WriteFile(filepath.Join(dest, name), body, 0o644); err != nil {
		return false, fmt.Errorf("fake-getter: write %s: %w", name, err)
	}

	return true, nil
}

// makeShaPointer builds a HuggingFace-format sha pointer file containing
// the oid sha256 and size lines for the supplied body. Mirrors the format
// CheckModel parses in sdk/kronk/model/check.go.
func makeShaPointer(body []byte) []byte {
	h := sha256.Sum256(body)
	return fmt.Appendf(nil,
		"version https://git-lfs.github.com/spec/v1\noid sha256:%x\nsize %d\n",
		h, len(body),
	)
}

// withFakeGetter swaps the package-level downloadFn / hasNetworkFn for
// the duration of the test, registering a Cleanup to restore them.
func withFakeGetter(t *testing.T, g *fakeGetter) {
	t.Helper()

	prevD := downloadFn
	prevN := hasNetworkFn

	downloadFn = g.download
	hasNetworkFn = func() bool { return true }

	t.Cleanup(func() {
		downloadFn = prevD
		hasNetworkFn = prevN
	})
}

// newTestModels constructs a Models with basePath under t.TempDir().
func newTestModels(t *testing.T) *Models {
	t.Helper()

	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	return m
}

var testLog applog.Logger = func(context.Context, string, ...any) {}

// =============================================================================
// downloadSplits / downloadModel coverage

func TestDownloadSplits_BareModel(t *testing.T) {
	body := []byte("body-bytes-for-Qwen3-0.6B-Q8_0\n")
	g := &fakeGetter{
		contents: map[string][]byte{
			"/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf": body,
		},
	}
	withFakeGetter(t, g)

	m := newTestModels(t)

	mp, err := m.downloadSplits(
		context.Background(), testLog,
		[]string{"https://huggingface.co/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"},
		"",
		"",
	)
	if err != nil {
		t.Fatalf("downloadSplits: %v", err)
	}

	if !mp.Downloaded {
		t.Error("Downloaded = false, want true")
	}
	if len(mp.ModelFiles) != 1 {
		t.Fatalf("ModelFiles = %v, want 1 entry", mp.ModelFiles)
	}
	if filepath.Base(mp.ModelFiles[0]) != "Qwen3-0.6B-Q8_0.gguf" {
		t.Errorf("ModelFiles[0] basename = %q", filepath.Base(mp.ModelFiles[0]))
	}
	if mp.ProjFile != "" {
		t.Errorf("ProjFile = %q, want empty", mp.ProjFile)
	}

	// Sha + model files should both be on disk.
	wantModel := filepath.Join(m.modelsPath, "Qwen", "Qwen3-0.6B-GGUF", "Qwen3-0.6B-Q8_0.gguf")
	if _, err := os.Stat(wantModel); err != nil {
		t.Errorf("model file missing: %v", err)
	}
	wantSha := filepath.Join(m.modelsPath, "Qwen", "Qwen3-0.6B-GGUF", "sha", "Qwen3-0.6B-Q8_0.gguf")
	if _, err := os.Stat(wantSha); err != nil {
		t.Errorf("sha file missing: %v", err)
	}
}

func TestDownloadSplits_WithProjection(t *testing.T) {
	body := []byte("model-body-bytes\n")
	proj := []byte("proj-body-bytes\n")

	g := &fakeGetter{
		contents: map[string][]byte{
			"/Qwen/Qwen3-VL-GGUF/resolve/main/Qwen3-VL-Q8_0.gguf": body,
			"/Qwen/Qwen3-VL-GGUF/resolve/main/mmproj-F16.gguf":    proj,
		},
	}
	withFakeGetter(t, g)

	m := newTestModels(t)

	mp, err := m.downloadSplits(
		context.Background(), testLog,
		[]string{"https://huggingface.co/Qwen/Qwen3-VL-GGUF/resolve/main/Qwen3-VL-Q8_0.gguf"},
		"https://huggingface.co/Qwen/Qwen3-VL-GGUF/resolve/main/mmproj-F16.gguf",
		"",
	)
	if err != nil {
		t.Fatalf("downloadSplits: %v", err)
	}

	if filepath.Base(mp.ProjFile) != "mmproj-Qwen3-VL-Q8_0.gguf" {
		t.Errorf("ProjFile basename = %q, want mmproj-Qwen3-VL-Q8_0.gguf (renamed from upstream mmproj-F16)", filepath.Base(mp.ProjFile))
	}
	if _, err := os.Stat(mp.ProjFile); err != nil {
		t.Errorf("renamed proj file missing: %v", err)
	}
}

func TestDownloadSplits_WithMTPCompanion(t *testing.T) {
	body := []byte("model-body-bytes\n")
	mtp := []byte("mtp-drafter-bytes\n")

	g := &fakeGetter{
		contents: map[string][]byte{
			"/unsloth/gemma-4-26B-A4B-it-GGUF/resolve/main/gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf": body,
			"/unsloth/gemma-4-26B-A4B-it-GGUF/resolve/main/mtp-gemma-4-26B-A4B-it.gguf":        mtp,
		},
	}
	withFakeGetter(t, g)

	m := newTestModels(t)

	mp, err := m.downloadSplits(
		context.Background(), testLog,
		[]string{"https://huggingface.co/unsloth/gemma-4-26B-A4B-it-GGUF/resolve/main/gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf"},
		"",
		"https://huggingface.co/unsloth/gemma-4-26B-A4B-it-GGUF/resolve/main/mtp-gemma-4-26B-A4B-it.gguf",
	)
	if err != nil {
		t.Fatalf("downloadSplits: %v", err)
	}

	// The MTP drafter is re-keyed to the main model id on disk.
	if filepath.Base(mp.MTPFile) != "mtp-gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf" {
		t.Errorf("MTPFile basename = %q, want mtp-gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf (renamed from upstream mtp-gemma-4-26B-A4B-it)", filepath.Base(mp.MTPFile))
	}
	if _, err := os.Stat(mp.MTPFile); err != nil {
		t.Errorf("renamed mtp file missing: %v", err)
	}

	// The companion must round-trip through the index onto the model's Path.
	fp, err := m.FullPath("gemma-4-26B-A4B-it-UD-Q8_K_XL")
	if err != nil {
		t.Fatalf("FullPath: %v", err)
	}
	if filepath.Base(fp.MTPFile) != "mtp-gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf" {
		t.Errorf("indexed MTPFile basename = %q, want mtp-gemma-4-26B-A4B-it-UD-Q8_K_XL.gguf", filepath.Base(fp.MTPFile))
	}
	if len(fp.ModelFiles) != 1 {
		t.Errorf("indexed ModelFiles = %v, want 1 (mtp companion must not be a standalone model)", fp.ModelFiles)
	}
}

func TestDownloadSplits_MultiShard(t *testing.T) {
	shard1 := []byte("shard-1-body-bytes\n")
	shard2 := []byte("shard-2-body-bytes\n")

	g := &fakeGetter{
		contents: map[string][]byte{
			"/unsloth/Llama-3.3-70B-Instruct-GGUF/resolve/main/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf": shard1,
			"/unsloth/Llama-3.3-70B-Instruct-GGUF/resolve/main/Llama-3.3-70B-Instruct-Q8_0-00002-of-00002.gguf": shard2,
		},
	}
	withFakeGetter(t, g)

	m := newTestModels(t)

	urls := []string{
		"https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/resolve/main/Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf",
		"https://huggingface.co/unsloth/Llama-3.3-70B-Instruct-GGUF/resolve/main/Llama-3.3-70B-Instruct-Q8_0-00002-of-00002.gguf",
	}

	mp, err := m.downloadSplits(context.Background(), testLog, urls, "", "")
	if err != nil {
		t.Fatalf("downloadSplits: %v", err)
	}

	if len(mp.ModelFiles) != 2 {
		t.Fatalf("ModelFiles = %v, want 2 shards", mp.ModelFiles)
	}

	wantBases := []string{
		"Llama-3.3-70B-Instruct-Q8_0-00001-of-00002.gguf",
		"Llama-3.3-70B-Instruct-Q8_0-00002-of-00002.gguf",
	}
	var gotBases []string
	for _, f := range mp.ModelFiles {
		gotBases = append(gotBases, filepath.Base(f))
	}
	if !reflect.DeepEqual(gotBases, wantBases) {
		t.Errorf("ModelFiles bases = %v, want %v", gotBases, wantBases)
	}
}

func TestDownloadSplits_IndexHit_SecondCallNoNetwork(t *testing.T) {
	body := []byte("body-for-index-hit\n")
	g := &fakeGetter{
		contents: map[string][]byte{
			"/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf": body,
		},
	}
	withFakeGetter(t, g)

	m := newTestModels(t)

	url := "https://huggingface.co/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"

	if _, err := m.downloadSplits(context.Background(), testLog, []string{url}, "", ""); err != nil {
		t.Fatalf("first downloadSplits: %v", err)
	}

	callsBefore := len(g.calls)

	// downloadSplits forces Downloaded=true at the end of the loop even
	// when every shard short-circuited on the index, so the aggregate
	// flag is not a reliable signal for "fetched any bytes". Assert via
	// the call count instead.
	if _, err := m.downloadSplits(context.Background(), testLog, []string{url}, "", ""); err != nil {
		t.Fatalf("second downloadSplits: %v", err)
	}

	if len(g.calls) != callsBefore {
		t.Errorf("expected no additional fake-getter calls on cache hit, got %d new (%v)", len(g.calls)-callsBefore, g.calls[callsBefore:])
	}
}

func TestDownloadSplits_IndexStale_FileDeleted(t *testing.T) {
	body := []byte("body-for-stale-test\n")
	g := &fakeGetter{
		contents: map[string][]byte{
			"/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf": body,
		},
	}
	withFakeGetter(t, g)

	m := newTestModels(t)

	url := "https://huggingface.co/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"

	mp, err := m.downloadSplits(context.Background(), testLog, []string{url}, "", "")
	if err != nil {
		t.Fatalf("first downloadSplits: %v", err)
	}

	// Delete the model file from disk — index still says validated.
	if err := os.Remove(mp.ModelFiles[0]); err != nil {
		t.Fatalf("rm model: %v", err)
	}

	callsBefore := len(g.calls)

	if _, err := m.downloadSplits(context.Background(), testLog, []string{url}, "", ""); err != nil {
		t.Fatalf("second downloadSplits: %v", err)
	}

	if len(g.calls) == callsBefore {
		t.Error("expected re-download after on-disk file removed; got no new fake-getter calls")
	}
}

func TestDownload_MissingCompanion_ReDownloads(t *testing.T) {
	g := &fakeGetter{
		contents: map[string][]byte{
			"/unsloth/gemma-4-26B-A4B-it-GGUF/resolve/main/gemma-4-26B-A4B-it-UD-Q4_K_M.gguf": []byte("model-body-bytes\n"),
			"/unsloth/gemma-4-26B-A4B-it-GGUF/resolve/main/mmproj-F16.gguf":                   []byte("proj-body-bytes\n"),
			"/unsloth/gemma-4-26B-A4B-it-GGUF/resolve/main/mtp-gemma-4-26B-A4B-it.gguf":       []byte("mtp-drafter-bytes\n"),
		},
	}
	withFakeGetter(t, g)

	m := newTestModels(t)

	// Seed the resolver catalog so Download resolves from cache without an
	// HF round-trip. The entry carries mmproj_orig/mtp_orig so the cache hit
	// can rebuild DownloadProj/DownloadMTP and never needs a repair search.
	catalogDir := filepath.Join(m.BasePath(), "catalog")
	if err := os.MkdirAll(catalogDir, 0o755); err != nil {
		t.Fatalf("mkdir catalog: %v", err)
	}
	mustWriteFile(t, filepath.Join(catalogDir, "catalog.yaml"), `schema: 1
providers:
  - unsloth
models:
  unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M:
    provider: unsloth
    family: gemma-4-26B-A4B-it-GGUF
    revision: main
    files:
      - gemma-4-26B-A4B-it-UD-Q4_K_M.gguf
    mmproj: mmproj-gemma-4-26B-A4B-it-UD-Q4_K_M.gguf
    mmproj_orig: mmproj-F16.gguf
    mtp: mtp-gemma-4-26B-A4B-it-UD-Q4_K_M.gguf
    mtp_orig: mtp-gemma-4-26B-A4B-it.gguf
    mtp_checked: true
`)

	canonical := "unsloth/gemma-4-26B-A4B-it-UD-Q4_K_M"

	// First pull installs the model body, projection, and MTP drafter.
	mp, err := m.Download(context.Background(), testLog, canonical)
	if err != nil {
		t.Fatalf("first Download: %v", err)
	}
	if mp.ProjFile == "" || mp.MTPFile == "" {
		t.Fatalf("first Download did not install companions: proj=%q mtp=%q", mp.ProjFile, mp.MTPFile)
	}
	for _, f := range []string{mp.ModelFiles[0], mp.ProjFile, mp.MTPFile} {
		if _, err := os.Stat(f); err != nil {
			t.Fatalf("expected %s on disk after first pull: %v", filepath.Base(f), err)
		}
	}

	// Regression: the user deletes only the companion files; the model body
	// stays. A second pull must NOT short-circuit on "already installed" —
	// it has to notice the catalog-tracked companions are gone and re-fetch.
	if err := os.Remove(mp.ProjFile); err != nil {
		t.Fatalf("rm proj: %v", err)
	}
	if err := os.Remove(mp.MTPFile); err != nil {
		t.Fatalf("rm mtp: %v", err)
	}

	callsBefore := len(g.calls)

	mp2, err := m.Download(context.Background(), testLog, canonical)
	if err != nil {
		t.Fatalf("second Download: %v", err)
	}

	if len(g.calls) == callsBefore {
		t.Fatal("expected re-download of missing companions; got no new fake-getter calls")
	}

	if _, err := os.Stat(mp2.ProjFile); err != nil {
		t.Errorf("projection not restored after re-download: %v", err)
	}
	if _, err := os.Stat(mp2.MTPFile); err != nil {
		t.Errorf("mtp drafter not restored after re-download: %v", err)
	}
}
