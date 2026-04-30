package models

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/hf"
	"go.yaml.in/yaml/v2"
)

// fakeHF is an in-memory HFClient for hermetic tests.
type fakeHF struct {
	// search maps "author|query" -> repos in order.
	search map[string][]string
	// metas maps "owner/repo" -> siblings.
	metas map[string][]string
	// missing repos return hf.ErrNotFound from ModelMeta.
	missing map[string]bool
	// hits records every Search/Meta call made for verification.
	calls []string
}

func (f *fakeHF) ModelMeta(_ context.Context, owner, repo, _ string) (hf.ModelMeta, error) {
	key := owner + "/" + repo
	f.calls = append(f.calls, "meta:"+key)
	if f.missing[key] {
		return hf.ModelMeta{}, hf.ErrNotFound
	}
	siblings, ok := f.metas[key]
	if !ok {
		return hf.ModelMeta{}, hf.ErrNotFound
	}
	return hf.ModelMeta{ID: key, Siblings: siblings}, nil
}

func (f *fakeHF) SearchModels(_ context.Context, author, query string) ([]string, error) {
	key := author + "|" + query
	f.calls = append(f.calls, "search:"+key)
	repos, ok := f.search[key]
	if !ok || len(repos) == 0 {
		return nil, hf.ErrNotFound
	}
	return repos, nil
}

func TestStripQuantSuffix(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Qwen3.6-35B-A3B-UD-Q4_K_M", "Qwen3.6-35B-A3B"},
		{"Qwen3.6-35B-A3B-Q4_K_M", "Qwen3.6-35B-A3B"},
		{"Qwen3.6-35B-A3B", "Qwen3.6-35B-A3B"},
		{"gemma-4-26B-A4B-it-UD-IQ3_M", "gemma-4-26B-A4B-it"},
		{"Llama-3.3-70B-Instruct-Q8_0-00001-of-00002", "Llama-3.3-70B-Instruct"},
		{"some-model-BF16", "some-model"},
		{"some-model-F16", "some-model"},
	}
	for _, tt := range tests {
		got := stripQuantSuffix(tt.in)
		if got != tt.want {
			t.Errorf("stripQuantSuffix(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestHasQuantSuffix(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"Qwen3.6-35B-A3B-UD-Q4_K_M", true},
		{"Qwen3.6-35B-A3B-Q4_K_M", true},
		{"Qwen3.6-35B-A3B", false},
		{"gemma-4-26B-A4B-it", false},
		{"Llama-3.3-70B-Instruct-Q8_0-00001-of-00002", true},
	}
	for _, tt := range tests {
		got := hasQuantSuffix(tt.in)
		if got != tt.want {
			t.Errorf("hasQuantSuffix(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestSelectFiles_ExactMatch(t *testing.T) {
	siblings := []string{
		"README.md",
		"Qwen3.6-35B-A3B-Q4_K_M.gguf",
		"Qwen3.6-35B-A3B-UD-Q4_K_M.gguf",
		"mmproj-F16.gguf",
		"mmproj-Q8_0.gguf",
	}

	files, mmproj, ok := selectFiles(siblings, "Qwen3.6-35B-A3B-UD-Q4_K_M")
	if !ok {
		t.Fatal("expected match")
	}
	if !reflect.DeepEqual(files, []string{"Qwen3.6-35B-A3B-UD-Q4_K_M.gguf"}) {
		t.Errorf("files = %v", files)
	}
	if mmproj != "mmproj-F16.gguf" {
		t.Errorf("mmproj = %q, want mmproj-F16.gguf", mmproj)
	}
}

func TestSelectFiles_NoQuantPrefersUD(t *testing.T) {
	siblings := []string{
		"Qwen3-Q4_K_M.gguf",
		"Qwen3-UD-Q4_K_M.gguf",
		"Qwen3-Q5_K_M.gguf",
	}
	files, _, ok := selectFiles(siblings, "Qwen3")
	if !ok {
		t.Fatal("expected match")
	}
	if !reflect.DeepEqual(files, []string{"Qwen3-UD-Q4_K_M.gguf"}) {
		t.Errorf("files = %v, want [Qwen3-UD-Q4_K_M.gguf]", files)
	}
}

func TestSelectFiles_NoQuantFallsBackToQ4KM(t *testing.T) {
	siblings := []string{
		"Qwen3-Q4_K_M.gguf",
		"Qwen3-Q5_K_M.gguf",
	}
	files, _, ok := selectFiles(siblings, "Qwen3")
	if !ok {
		t.Fatal("expected match")
	}
	if !reflect.DeepEqual(files, []string{"Qwen3-Q4_K_M.gguf"}) {
		t.Errorf("files = %v", files)
	}
}

func TestSelectFiles_NoMatch(t *testing.T) {
	siblings := []string{
		"Qwen3-Q5_K_M.gguf",
		"Qwen3-Q8_0.gguf",
	}
	if _, _, ok := selectFiles(siblings, "Qwen3"); ok {
		t.Fatal("expected no match (no Q4_K_M variant)")
	}
}

func TestSelectFiles_Split(t *testing.T) {
	siblings := []string{
		"Llama-3.3-70B-Q8_0/Llama-3.3-70B-Q8_0-00001-of-00002.gguf",
		"Llama-3.3-70B-Q8_0/Llama-3.3-70B-Q8_0-00002-of-00002.gguf",
	}
	files, _, ok := selectFiles(siblings, "Llama-3.3-70B-Q8_0")
	if !ok {
		t.Fatal("expected match")
	}
	want := []string{
		"Llama-3.3-70B-Q8_0/Llama-3.3-70B-Q8_0-00001-of-00002.gguf",
		"Llama-3.3-70B-Q8_0/Llama-3.3-70B-Q8_0-00002-of-00002.gguf",
	}
	if !reflect.DeepEqual(files, want) {
		t.Errorf("files = %v, want %v", files, want)
	}
}

func TestSelectFiles_MmprojOnlyF16(t *testing.T) {
	siblings := []string{
		"Qwen-Q4_K_M.gguf",
		"mmproj-Q8_0.gguf",
	}
	_, mmproj, ok := selectFiles(siblings, "Qwen-Q4_K_M")
	if !ok {
		t.Fatal("expected match")
	}
	if mmproj != "" {
		t.Errorf("mmproj = %q, want empty (no F16 available)", mmproj)
	}
}

func TestSelectFiles_MmprojRejectsBF16(t *testing.T) {
	// BF16 is not F16. When only BF16 is published the projection
	// should be left empty rather than mis-selected.
	siblings := []string{
		"gemma-Q4_K_M.gguf",
		"mmproj-BF16.gguf",
	}
	_, mmproj, ok := selectFiles(siblings, "gemma-Q4_K_M")
	if !ok {
		t.Fatal("expected match")
	}
	if mmproj != "" {
		t.Errorf("mmproj = %q, want empty (BF16 must not match F16)", mmproj)
	}
}

func TestSelectFiles_MmprojPrefersF16OverOthers(t *testing.T) {
	siblings := []string{
		"gemma-Q4_K_M.gguf",
		"mmproj-BF16.gguf",
		"mmproj-F16.gguf",
		"mmproj-F32.gguf",
	}
	_, mmproj, ok := selectFiles(siblings, "gemma-Q4_K_M")
	if !ok {
		t.Fatal("expected match")
	}
	if mmproj != "mmproj-F16.gguf" {
		t.Errorf("mmproj = %q, want mmproj-F16.gguf", mmproj)
	}
}

func TestResolver_HFHit_PersistsAndReturnsURLs(t *testing.T) {
	hf := &fakeHF{
		search: map[string][]string{
			"unsloth|Qwen3.6-35B-A3B": {"unsloth/Qwen3.6-35B-A3B-GGUF"},
		},
		metas: map[string][]string{
			"unsloth/Qwen3.6-35B-A3B-GGUF": {
				"Qwen3.6-35B-A3B-Q4_K_M.gguf",
				"Qwen3.6-35B-A3B-UD-Q4_K_M.gguf",
				"mmproj-F16.gguf",
			},
		},
	}

	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers:\n  - unsloth\n  - ggml-org\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hf)

	res, err := r.Resolve(context.Background(), "Qwen3.6-35B-A3B-UD-Q4_K_M")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if res.CanonicalID != "unsloth/Qwen3.6-35B-A3B-UD-Q4_K_M" {
		t.Errorf("CanonicalID = %q", res.CanonicalID)
	}
	if res.Provider != "unsloth" || res.Family != "Qwen3.6-35B-A3B-GGUF" {
		t.Errorf("provider/family = %q/%q", res.Provider, res.Family)
	}
	if !reflect.DeepEqual(res.Files, []string{"Qwen3.6-35B-A3B-UD-Q4_K_M.gguf"}) {
		t.Errorf("Files = %v", res.Files)
	}
	if res.MMProj != "mmproj-Qwen3.6-35B-A3B-UD-Q4_K_M.gguf" {
		t.Errorf("MMProj = %q", res.MMProj)
	}
	if res.MMProjOrig != "mmproj-F16.gguf" {
		t.Errorf("MMProjOrig = %q", res.MMProjOrig)
	}
	if got, want := res.DownloadURLs[0], "https://huggingface.co/unsloth/Qwen3.6-35B-A3B-GGUF/resolve/main/Qwen3.6-35B-A3B-UD-Q4_K_M.gguf"; got != want {
		t.Errorf("DownloadURLs[0] = %q, want %q", got, want)
	}
	if !strings.Contains(res.DownloadProj, "mmproj-F16.gguf") {
		t.Errorf("DownloadProj = %q", res.DownloadProj)
	}

	// Verify the file was persisted.
	persisted := loadResolved(t, rfile)
	entry, ok := persisted.Models["unsloth/Qwen3.6-35B-A3B-UD-Q4_K_M"]
	if !ok {
		t.Fatal("entry not persisted")
	}
	if entry.Provider != "unsloth" || entry.Family != "Qwen3.6-35B-A3B-GGUF" {
		t.Errorf("persisted entry wrong: %+v", entry)
	}
}

func TestResolver_ProviderWalk_StopsAtFirstHit(t *testing.T) {
	hf := &fakeHF{
		search: map[string][]string{
			"unsloth|Qwen3":  {}, // empty -> hf.ErrNotFound
			"ggml-org|Qwen3": {"ggml-org/Qwen3-GGUF"},
		},
		metas: map[string][]string{
			"ggml-org/Qwen3-GGUF": {"Qwen3-Q4_K_M.gguf"},
		},
	}
	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers:\n  - unsloth\n  - ggml-org\n  - bartowski\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hf)

	res, err := r.Resolve(context.Background(), "Qwen3")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if res.Provider != "ggml-org" {
		t.Errorf("Provider = %q, want ggml-org", res.Provider)
	}

	// We should not have queried bartowski.
	for _, c := range hf.calls {
		if strings.HasPrefix(c, "search:bartowski") {
			t.Errorf("unexpectedly queried bartowski: %v", hf.calls)
		}
	}
}

func TestResolver_ExplicitProvider(t *testing.T) {
	hf := &fakeHF{
		search: map[string][]string{
			"bartowski|Foo": {"bartowski/Foo-GGUF"},
		},
		metas: map[string][]string{
			"bartowski/Foo-GGUF": {"Foo-Q4_K_M.gguf"},
		},
	}
	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers:\n  - unsloth\n  - ggml-org\n  - bartowski\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hf)

	res, err := r.Resolve(context.Background(), "bartowski/Foo")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.CanonicalID != "bartowski/Foo" {
		t.Errorf("CanonicalID = %q", res.CanonicalID)
	}

	// Confirm we never asked unsloth or ggml-org.
	for _, c := range hf.calls {
		if strings.HasPrefix(c, "search:unsloth") || strings.HasPrefix(c, "search:ggml-org") {
			t.Errorf("explicit provider did not skip others: %v", hf.calls)
		}
	}
}

func TestResolver_CacheHitNoHFCall(t *testing.T) {
	hf := &fakeHF{}
	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")

	cached := Catalog{
		Providers: []string{"unsloth"},
		Models: map[string]CatalogEntry{
			"unsloth/Qwen3-Q4_K_M": {
				Provider:   "unsloth",
				Family:     "Qwen3-GGUF",
				Revision:   "main",
				Files:      []string{"Qwen3-Q4_K_M.gguf"},
				MMProj:     "mmproj-Qwen3-Q4_K_M.gguf",
				MMProjOrig: "mmproj-F16.gguf",
			},
		},
	}
	data, _ := yaml.Marshal(cached)
	mustWriteFile(t, rfile, string(data))

	r := NewResolverWithClient(nil, rfile, hf)

	res, err := r.Resolve(context.Background(), "Qwen3-Q4_K_M")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !res.FromCache {
		t.Error("expected FromCache=true")
	}
	if len(hf.calls) > 0 {
		t.Errorf("expected zero HF calls, got %v", hf.calls)
	}
	if !reflect.DeepEqual(res.Files, []string{"Qwen3-Q4_K_M.gguf"}) {
		t.Errorf("Files = %v", res.Files)
	}
}

func TestResolver_NotFoundAcrossProviders(t *testing.T) {
	hf := &fakeHF{}
	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers:\n  - unsloth\n  - ggml-org\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hf)

	_, err := r.Resolve(context.Background(), "DoesNotExist")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want a 'not found' message", err)
	}
}

func TestResolver_HFNotFoundIsNotFatal(t *testing.T) {
	// Ensure the resolver treats hf.ErrNotFound from one provider as a
	// "skip" rather than a hard error.
	hf := &fakeHF{
		search: map[string][]string{
			"ggml-org|Qwen3": {"ggml-org/Qwen3-GGUF"},
		},
		metas: map[string][]string{
			"ggml-org/Qwen3-GGUF": {"Qwen3-Q4_K_M.gguf"},
		},
	}
	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers:\n  - unsloth\n  - ggml-org\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hf)

	res, err := r.Resolve(context.Background(), "Qwen3")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if res.Provider != "ggml-org" {
		t.Errorf("Provider = %q", res.Provider)
	}
}

func TestErrNotFoundDetection(t *testing.T) {
	if !isNotFound(hf.ErrNotFound) {
		t.Error("isNotFound did not detect hf.ErrNotFound")
	}
	wrapped := errors.New("oh: " + hf.ErrNotFound.Error())
	if !isNotFound(wrapped) {
		t.Error("isNotFound did not detect wrapped err")
	}
	if isNotFound(errors.New("other")) {
		t.Error("isNotFound matched unrelated error")
	}
}

// =============================================================================

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func loadResolved(t *testing.T, path string) Catalog {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var rm Catalog
	if err := yaml.Unmarshal(b, &rm); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	// Ensure deterministic provider ordering for any test that inspects it.
	sort.Strings(rm.Providers)
	return rm
}
