package models

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
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

func TestFullPathNotFound(t *testing.T) {
	var m Models

	_, err := m.FullPath("provider/missing")
	if !errors.Is(err, ErrModelNotFound) {
		t.Fatalf("FullPath: got %v, want %v", err, ErrModelNotFound)
	}
}

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
	fp, err := m.FullPath("unsloth/gemma-4-26B-A4B-it-UD-Q8_K_XL")
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
	mustWriteFile(t, filepath.Join(catalogDir, "catalog.yaml"), `models:
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

// TestDownloadSplits_CompanionRenamedByPeer reproduces the shared-models-
// directory race that broke a macOS CI job: two runners pointed at one
// models mount pulled the same model seconds apart, and the second one
// died with
//
//	unable to rename proj file: rename .../X.mmproj-f16.gguf
//	  .../mmproj-X.Q4_K_M.gguf: no such file or directory
//
// because the first had already renamed the upstream-named file to the
// canonical one. The peer's file is complete and verifiable, so the loser
// must adopt it rather than fail. Both rename sites in downloadCompanion
// are exercised: the sha pointer and the companion body.
func TestDownloadSplits_CompanionRenamedByPeer(t *testing.T) {
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
	dir := filepath.Join(m.modelsPath, "Qwen", "Qwen3-VL-GGUF")

	// Stand in for the competing process: the instant this one has pulled a
	// projection artifact, move it to the canonical name the peer's own
	// rename would have used. That leaves our rename with no source and a
	// finished destination — the exact state CI hit.
	pull := downloadFn
	downloadFn = func(ctx context.Context, src string, dest string, p downloader.ProgressFunc, interval int64) (bool, error) {
		downloaded, err := pull(ctx, src, dest, p, interval)
		if err != nil || !strings.Contains(src, "mmproj-F16.gguf") {
			return downloaded, err
		}

		from, to := filepath.Join(dir, "mmproj-F16.gguf"), filepath.Join(dir, "mmproj-Qwen3-VL-Q8_0.gguf")
		if strings.Contains(src, "/raw/") {
			from, to = filepath.Join(dir, "sha", "mmproj-F16.gguf"), filepath.Join(dir, "sha", "mmproj-Qwen3-VL-Q8_0.gguf")
		}
		if err := os.Rename(from, to); err != nil {
			return false, fmt.Errorf("peer rename: %w", err)
		}

		return downloaded, nil
	}

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
		t.Errorf("ProjFile basename = %q, want mmproj-Qwen3-VL-Q8_0.gguf", filepath.Base(mp.ProjFile))
	}

	got, err := os.ReadFile(mp.ProjFile)
	if err != nil {
		t.Fatalf("read adopted proj file: %v", err)
	}
	if !bytes.Equal(got, proj) {
		t.Errorf("adopted proj file = %q, want %q", got, proj)
	}
}

// TestDownloadSplits_CompanionRenameFailureStaysFatal is the other half of
// TestDownloadSplits_CompanionRenamedByPeer: adoption is keyed on the
// source having vanished, so a destination that happens to exist must not
// paper over a rename that failed for any other reason.
func TestDownloadSplits_CompanionRenameFailureStaysFatal(t *testing.T) {
	if adoptedFromPeer(errors.New("read-only file system"), ".", nil) {
		t.Error("adoptedFromPeer accepted a non-ENOENT rename failure")
	}
}

// =============================================================================
// pull — oversized destination handling

// TestPullBody_RemovesOversizedDestination covers the one on-disk state the
// getter cannot recover from on its own: a body longer than the size its sha
// pointer records. The getter treats such a destination as already complete
// and returns it untouched, so unless the pull clears it first the file fails
// its size check forever.
func TestPullBody_RemovesOversizedDestination(t *testing.T) {
	body := []byte("body-bytes-for-Qwen3-0.6B-Q8_0\n")
	rawURL := "https://huggingface.co/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"

	m := newTestModels(t)

	loc, err := newLocator(rawURL)
	if err != nil {
		t.Fatalf("newLocator: %v", err)
	}

	destFile := loc.ModelPath(m)
	shaFile := filepath.Join(filepath.Dir(destFile), "sha", filepath.Base(destFile))

	if err := os.MkdirAll(filepath.Dir(shaFile), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(shaFile, makeShaPointer(body), 0o644); err != nil {
		t.Fatalf("write sha pointer: %v", err)
	}

	// Two writers appending into one destination: the pointer says len(body),
	// the file holds twice that.
	if err := os.WriteFile(destFile, append(append([]byte(nil), body...), body...), 0o644); err != nil {
		t.Fatalf("write oversized body: %v", err)
	}

	var sawDestination bool

	prevD, prevN := downloadFn, hasNetworkFn
	downloadFn = func(ctx context.Context, src string, dest string, p downloader.ProgressFunc, interval int64) (bool, error) {
		_, err := os.Stat(destFile)
		sawDestination = err == nil

		return true, nil
	}
	hasNetworkFn = func() bool { return true }

	t.Cleanup(func() {
		downloadFn = prevD
		hasNetworkFn = prevN
	})

	if _, _, err := m.pull(context.Background(), loc, pullBody, nil); err != nil {
		t.Fatalf("pull: %v", err)
	}

	if sawDestination {
		t.Error("oversized body survived into the download; want it removed so the pull refetches from scratch")
	}
}

// TestRemoveOversizedBody_KeepsEverythingElse pins the states that must
// survive: a short file is exactly what the getter's resume exists for, and a
// body with no readable pointer has no expected size to be measured against.
func TestRemoveOversizedBody_KeepsEverythingElse(t *testing.T) {
	body := []byte("body-bytes-for-Qwen3-0.6B-Q8_0\n")

	tests := []struct {
		name    string
		content []byte
		pointer []byte
		want    bool // file still on disk after the call
	}{
		{"short", body[:10], makeShaPointer(body), true},
		{"exact", body, makeShaPointer(body), true},
		{"oversized", append(append([]byte(nil), body...), 'x'), makeShaPointer(body), false},
		{"no pointer", append(append([]byte(nil), body...), 'x'), nil, true},
		{"garbled pointer", append(append([]byte(nil), body...), 'x'), []byte("not a pointer\n"), true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			destFile := filepath.Join(dir, "Qwen3-0.6B-Q8_0.gguf")
			shaFile := filepath.Join(dir, "sha", filepath.Base(destFile))

			if err := os.MkdirAll(filepath.Dir(shaFile), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if test.pointer != nil {
				if err := os.WriteFile(shaFile, test.pointer, 0o644); err != nil {
					t.Fatalf("write sha pointer: %v", err)
				}
			}
			if err := os.WriteFile(destFile, test.content, 0o644); err != nil {
				t.Fatalf("write body: %v", err)
			}

			if err := removeOversizedBody(destFile, shaFile); err != nil {
				t.Fatalf("removeOversizedBody: %v", err)
			}

			_, err := os.Stat(destFile)
			if got := err == nil; got != test.want {
				t.Errorf("body on disk = %v, want %v", got, test.want)
			}
		})
	}

	// A destination that is not there at all is the common case on a cold
	// pull and must not be an error.
	absent := filepath.Join(t.TempDir(), "absent.gguf")
	if err := removeOversizedBody(absent, artifactDigestPath(absent)); err != nil {
		t.Errorf("removeOversizedBody on a missing file: %v", err)
	}
}

// TestDownloadSplits_OversizedCompanionLeftover is the companion half of the
// same brick. A prior run left the upstream-named projection on disk longer
// than its pointer says (two writers appending into one destination), and the
// getter hands back such a file untouched. Unless the download clears it, the
// oversized leftover is renamed over the canonical name and fails its sha
// check on this run and every run after it.
func TestDownloadSplits_OversizedCompanionLeftover(t *testing.T) {
	body := []byte("body-bytes-for-Qwen3-VL-Q8_0\n")
	proj := []byte("proj-bytes-for-Qwen3-VL-Q8_0\n")

	g := &fakeGetter{
		contents: map[string][]byte{
			"/Qwen/Qwen3-VL-GGUF/resolve/main/Qwen3-VL-Q8_0.gguf": body,
			"/Qwen/Qwen3-VL-GGUF/resolve/main/mmproj-F16.gguf":    proj,
		},
	}
	withFakeGetter(t, g)

	m := newTestModels(t)
	dir := filepath.Join(m.modelsPath, "Qwen", "Qwen3-VL-GGUF")
	leftover := filepath.Join(dir, "mmproj-F16.gguf")

	if err := os.MkdirAll(filepath.Join(dir, "sha"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(leftover, append(append([]byte(nil), proj...), proj...), 0o644); err != nil {
		t.Fatalf("write oversized leftover: %v", err)
	}

	// The state a killed run really leaves: the pointer made it to its
	// canonical name, the body did not, so the leftover carries the upstream
	// name and there is a pointer to measure it against.
	canonicalSha := filepath.Join(dir, "sha", "mmproj-Qwen3-VL-Q8_0.gguf")
	if err := os.WriteFile(canonicalSha, makeShaPointer(proj), 0o644); err != nil {
		t.Fatalf("write canonical sha pointer: %v", err)
	}

	// The fake getter always writes; the real one does not. Stand in for
	// go-getter's actual behavior on the one URL that matters here: a
	// destination at or past the expected length is left exactly as it is.
	pull := downloadFn
	downloadFn = func(ctx context.Context, src string, dest string, p downloader.ProgressFunc, interval int64) (bool, error) {
		if strings.Contains(src, "mmproj-F16.gguf") && !strings.Contains(src, "/raw/") {
			if fi, err := os.Stat(leftover); err == nil && fi.Size() >= int64(len(proj)) {
				return false, nil
			}
		}

		return pull(ctx, src, dest, p, interval)
	}

	mp, err := m.downloadSplits(
		context.Background(), testLog,
		[]string{"https://huggingface.co/Qwen/Qwen3-VL-GGUF/resolve/main/Qwen3-VL-Q8_0.gguf"},
		"https://huggingface.co/Qwen/Qwen3-VL-GGUF/resolve/main/mmproj-F16.gguf",
		"",
	)
	if err != nil {
		t.Fatalf("downloadSplits: %v", err)
	}

	got, err := os.ReadFile(mp.ProjFile)
	if err != nil {
		t.Fatalf("read proj file: %v", err)
	}
	if !bytes.Equal(got, proj) {
		t.Errorf("proj file = %q, want %q", got, proj)
	}
}

// =============================================================================
// adopt-or-download decisions — unverifiable is not verified

// TestDownloadCompanion_ReuseByURLNameRequiresPointer covers the reuse
// shortcut in tryReuseCompanionFromURLName. A previous pull can leave the
// companion under its upstream name ("mmproj-F16.gguf"), and the shortcut
// copies that file over the canonical name rather than fetch the body again.
// That is only a shortcut when the leftover can actually be verified: the
// sha pointer is copied along with the body *if one exists*, so a leftover
// with no pointer under either name reached a check with nothing to check
// against — which the lenient checkModel reported as success. An arbitrary,
// truncated, or oversized body was then adopted as the finished companion
// and the real one never downloaded.
//
// The third case is the other half of the rule: a leftover that does verify
// must still short-circuit the body download, or the optimization is gone.
func TestDownloadCompanion_ReuseByURLNameRequiresPointer(t *testing.T) {
	proj := []byte("proj-bytes-for-Qwen3-VL-Q8_0\n")

	const (
		projURL  = "https://huggingface.co/Qwen/Qwen3-VL-GGUF/resolve/main/mmproj-F16.gguf"
		projPath = "/Qwen/Qwen3-VL-GGUF/resolve/main/mmproj-F16.gguf"
	)

	tests := []struct {
		name         string
		leftover     []byte
		pointer      []byte // written beside the leftover under its upstream name; nil = none
		wantBodyPull bool
	}{
		{
			name:         "no pointer to verify against",
			leftover:     bytes.Repeat([]byte("x"), 2*len(proj)),
			pointer:      nil,
			wantBodyPull: true,
		},
		{
			name:         "pointer describes different bytes",
			leftover:     bytes.Repeat([]byte("x"), len(proj)),
			pointer:      makeShaPointer([]byte("a different companion\n")),
			wantBodyPull: true,
		},
		{
			name:         "verifiable leftover is reused",
			leftover:     proj,
			pointer:      makeShaPointer(proj),
			wantBodyPull: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := &fakeGetter{contents: map[string][]byte{projPath: proj}}
			withFakeGetter(t, g)

			m := newTestModels(t)
			dir := filepath.Join(m.modelsPath, "Qwen", "Qwen3-VL-GGUF")
			if err := os.MkdirAll(filepath.Join(dir, "sha"), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			// The state a killed pull (or a hand-rolled wget) leaves behind:
			// the companion under its upstream name, with or without a
			// pointer beside it.
			if err := os.WriteFile(filepath.Join(dir, "mmproj-F16.gguf"), test.leftover, 0o644); err != nil {
				t.Fatalf("write leftover: %v", err)
			}
			if test.pointer != nil {
				if err := os.WriteFile(filepath.Join(dir, "sha", "mmproj-F16.gguf"), test.pointer, 0o644); err != nil {
					t.Fatalf("write leftover pointer: %v", err)
				}
			}

			loc, err := newLocator(projURL)
			if err != nil {
				t.Fatalf("newLocator: %v", err)
			}

			got, fetched, err := m.downloadCompanion(context.Background(), testLog, loc, filepath.Join(dir, "Qwen3-VL-Q8_0.gguf"), companionProj, nil)
			if err != nil {
				t.Fatalf("downloadCompanion: %v", err)
			}

			if filepath.Base(got) != "mmproj-Qwen3-VL-Q8_0.gguf" {
				t.Errorf("companion path = %q, want mmproj-Qwen3-VL-Q8_0.gguf", filepath.Base(got))
			}
			if fetched != test.wantBodyPull {
				t.Errorf("fetched = %v, want %v", fetched, test.wantBodyPull)
			}

			var bodyPulls int
			for _, call := range g.calls {
				if strings.Contains(call, "mmproj-F16.gguf") && !strings.Contains(call, "/raw/") {
					bodyPulls++
				}
			}
			if (bodyPulls > 0) != test.wantBodyPull {
				t.Errorf("body pulls = %d, want any = %v (calls: %v)", bodyPulls, test.wantBodyPull, g.calls)
			}

			// Whatever route was taken, the installed companion has to be
			// the upstream file — never the leftover that could not be
			// verified.
			content, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("read companion: %v", err)
			}
			if !bytes.Equal(content, proj) {
				t.Errorf("companion content = %q, want %q", content, proj)
			}
		})
	}
}

// TestDownloadModelFile_UnverifiableBodyIsNotAdopted pins the same rule for
// the model body. pull reports downloaded==false when the getter decided the
// destination was already complete and left it untouched — downloader.Download
// maps "no bytes transferred" onto exactly that — so the check after the pull
// is the only thing standing between a leftover body and being reported as
// the installed model. When there is no pointer on disk to check it against
// (a peer's `kronk model remove` clearing the sha directory between the two
// pulls, a mirror that answers the pointer request with nothing) the lenient
// check passed and an unverified body was installed as the model.
//
// A body that cannot be verified must fail the pull, not pass it: unlike a
// reuse site there is no shortcut to fall through here, the download already
// happened.
func TestDownloadModelFile_UnverifiableBodyIsNotAdopted(t *testing.T) {
	body := []byte("body-bytes-for-Qwen3-0.6B-Q8_0\n")
	leftover := bytes.Repeat([]byte("x"), len(body))

	const (
		modelURL  = "https://huggingface.co/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"
		modelPath = "/Qwen/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"
	)

	g := &fakeGetter{contents: map[string][]byte{modelPath: body}}
	withFakeGetter(t, g)

	m := newTestModels(t)
	dir := filepath.Join(m.modelsPath, "Qwen", "Qwen3-0.6B-GGUF")
	destFile := filepath.Join(dir, "Qwen3-0.6B-Q8_0.gguf")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(destFile, leftover, 0o644); err != nil {
		t.Fatalf("write leftover body: %v", err)
	}

	// Stand in for the getter's "nothing to transfer" answer on both
	// artifacts: the pointer never reaches disk, and the body already there
	// is left exactly as it was found.
	pull := downloadFn
	downloadFn = func(ctx context.Context, src string, dest string, p downloader.ProgressFunc, interval int64) (bool, error) {
		if strings.Contains(src, "/raw/") {
			return false, nil
		}
		if _, err := os.Stat(destFile); err == nil {
			return false, nil
		}

		return pull(ctx, src, dest, p, interval)
	}
	t.Cleanup(func() { downloadFn = pull })

	if _, err := m.downloadSplits(context.Background(), testLog, []string{modelURL}, "", ""); err == nil {
		t.Fatal("downloadSplits: got nil, want a failure — nothing on disk could verify the model body")
	}

	if _, mp, found := lookupIndex(m.loadIndex(), canonicalID("Qwen", "Qwen3-0.6B-Q8_0")); found && mp.Validated {
		t.Error("model marked validated even though nothing verified the body on disk")
	}
}

// TestDownloadCompanion_ReuseBySHAStillShortCircuits guards the other two
// reuse decisions, in tryReuseCompanionFromSHA, against the same tightening:
// both are strict now, and both must still take their shortcut. A companion
// whose pointer matches the freshly pulled one is verifiable by construction
// — the pointer is either already at the canonical path (the in-place adopt,
// existingName == the canonical name) or copied there before the check (the
// cross-id copy) — so strictness may not cost a body download here.
func TestDownloadCompanion_ReuseBySHAStillShortCircuits(t *testing.T) {
	proj := []byte("proj-bytes-shared-across-quants\n")

	const (
		projURL  = "https://huggingface.co/Qwen/Qwen3-VL-GGUF/resolve/main/mmproj-F16.gguf"
		projPath = "/Qwen/Qwen3-VL-GGUF/resolve/main/mmproj-F16.gguf"
	)

	tests := []struct {
		name         string
		existingName string
	}{
		{"cross-id copy", "mmproj-Qwen3-VL-Q4_K_M.gguf"},
		{"adopt in place", "mmproj-Qwen3-VL-Q8_0.gguf"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			g := &fakeGetter{contents: map[string][]byte{projPath: proj}}
			withFakeGetter(t, g)

			m := newTestModels(t)
			dir := filepath.Join(m.modelsPath, "Qwen", "Qwen3-VL-GGUF")
			if err := os.MkdirAll(filepath.Join(dir, "sha"), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}

			// The identical companion, already installed and verifiable —
			// either under another quant's id or under this one's.
			if err := os.WriteFile(filepath.Join(dir, test.existingName), proj, 0o644); err != nil {
				t.Fatalf("write existing companion: %v", err)
			}
			if err := os.WriteFile(filepath.Join(dir, "sha", test.existingName), makeShaPointer(proj), 0o644); err != nil {
				t.Fatalf("write existing pointer: %v", err)
			}

			loc, err := newLocator(projURL)
			if err != nil {
				t.Fatalf("newLocator: %v", err)
			}

			got, fetched, err := m.downloadCompanion(context.Background(), testLog, loc, filepath.Join(dir, "Qwen3-VL-Q8_0.gguf"), companionProj, nil)
			if err != nil {
				t.Fatalf("downloadCompanion: %v", err)
			}

			if fetched {
				t.Error("fetched = true, want false — the companion on disk matches the upstream pointer")
			}
			for _, call := range g.calls {
				if strings.Contains(call, "mmproj-F16.gguf") && !strings.Contains(call, "/raw/") {
					t.Errorf("companion body was pulled despite a verifiable local match (calls: %v)", g.calls)
				}
			}

			content, err := os.ReadFile(got)
			if err != nil {
				t.Fatalf("read companion: %v", err)
			}
			if !bytes.Equal(content, proj) {
				t.Errorf("companion content = %q, want %q", content, proj)
			}
			if _, err := os.Stat(filepath.Join(dir, "sha", "mmproj-Qwen3-VL-Q8_0.gguf")); err != nil {
				t.Errorf("canonical sha pointer missing after reuse: %v", err)
			}
			if _, err := os.Stat(filepath.Join(dir, "sha", "mmproj-F16.gguf")); err == nil {
				t.Error("upstream-named sha pointer left behind after a verified reuse")
			}
		})
	}
}
