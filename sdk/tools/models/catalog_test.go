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
		{"Qwen2-Audio-7B.Q8_0", "Qwen2-Audio-7B"},
		{"Qwen2-Audio-7B.Q4_K_M", "Qwen2-Audio-7B"},
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
		{"Qwen2-Audio-7B.Q8_0", true},
		{"Qwen2-Audio-7B.Q4_K_M", true},
	}
	for _, tt := range tests {
		got := hasQuantSuffix(tt.in)
		if got != tt.want {
			t.Errorf("hasQuantSuffix(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestExtractQuantTag(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"Qwen3.6-35B-A3B-UD-Q8_K_XL", "UD-Q8_K_XL"},
		{"Qwen3.6-35B-A3B-UD-Q4_K_M", "UD-Q4_K_M"},
		{"Qwen3.6-35B-A3B-Q4_K_M", "Q4_K_M"},
		{"gemma-4-26B-A4B-it-UD-IQ3_M", "UD-IQ3_M"},
		{"Llama-3.3-70B-Instruct-Q8_0-00001-of-00002", "Q8_0"},
		{"some-model-BF16", "BF16"},
		{"some-model-F16", "F16"},
		{"Qwen2-Audio-7B.Q8_0", "Q8_0"},
		{"Qwen3.6-35B-A3B", ""},
		{"gemma-4-26B-A4B-it", ""},
	}
	for _, tt := range tests {
		got := extractQuantTag(tt.in)
		if got != tt.want {
			t.Errorf("extractQuantTag(%q) = %q, want %q", tt.in, got, tt.want)
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

func TestSelectFiles_MmprojFallsBackWhenNoF16(t *testing.T) {
	// mradermacher and similar quantizers publish only quantized mmproj
	// variants. Falling back to the highest-quality available quant lets
	// these models work end-to-end instead of silently disabling media
	// support.
	siblings := []string{
		"Qwen-Q4_K_M.gguf",
		"mmproj-Q8_0.gguf",
	}
	_, mmproj, ok := selectFiles(siblings, "Qwen-Q4_K_M")
	if !ok {
		t.Fatal("expected match")
	}
	if mmproj != "mmproj-Q8_0.gguf" {
		t.Errorf("mmproj = %q, want mmproj-Q8_0.gguf (F16 absent — best quant fallback)", mmproj)
	}
}

func TestSelectFiles_MmprojEmbeddedNamingMradermacher(t *testing.T) {
	// mradermacher prefixes every artifact with the model id, including
	// the projection: "<id>.mmproj-<quant>.gguf". Earlier code only
	// recognized files starting with "mmproj", so these were
	// misclassified as model files and the projection was silently lost.
	siblings := []string{
		"Qwen2-Audio-7B.Q8_0.gguf",
		"Qwen2-Audio-7B.mmproj-Q8_0.gguf",
		"Qwen2-Audio-7B.mmproj-f16.gguf",
	}
	files, mmproj, ok := selectFiles(siblings, "Qwen2-Audio-7B.Q8_0")
	if !ok {
		t.Fatal("expected match")
	}
	if !reflect.DeepEqual(files, []string{"Qwen2-Audio-7B.Q8_0.gguf"}) {
		t.Errorf("files = %v, want [Qwen2-Audio-7B.Q8_0.gguf] (mmproj must not leak into model files)", files)
	}
	if mmproj != "Qwen2-Audio-7B.mmproj-f16.gguf" {
		t.Errorf("mmproj = %q, want Qwen2-Audio-7B.mmproj-f16.gguf", mmproj)
	}
}

func TestSelectFiles_MmprojBF16NotMisclassifiedAsF16(t *testing.T) {
	// BF16 is not F16. The F16 regex must not match BF16. When only
	// BF16 is available it is now accepted as a fallback projection
	// (better than no media support), but it must never be reported as
	// the F16 selection.
	siblings := []string{
		"gemma-Q4_K_M.gguf",
		"mmproj-BF16.gguf",
		"mmproj-F16.gguf",
	}
	_, mmproj, ok := selectFiles(siblings, "gemma-Q4_K_M")
	if !ok {
		t.Fatal("expected match")
	}
	if mmproj != "mmproj-F16.gguf" {
		t.Errorf("mmproj = %q, want mmproj-F16.gguf (must prefer F16 over BF16)", mmproj)
	}
}

func TestSelectFiles_MmprojBF16FallbackAcceptedWhenAlone(t *testing.T) {
	siblings := []string{
		"gemma-Q4_K_M.gguf",
		"mmproj-BF16.gguf",
	}
	_, mmproj, ok := selectFiles(siblings, "gemma-Q4_K_M")
	if !ok {
		t.Fatal("expected match")
	}
	if mmproj != "mmproj-BF16.gguf" {
		t.Errorf("mmproj = %q, want mmproj-BF16.gguf (only candidate available)", mmproj)
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

func TestNeedsParse(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		// Bare ids and canonical ids stay on the resolver path.
		{"Qwen3-0.6B-Q8_0", false},
		{"unsloth/Qwen3-0.6B-Q8_0", false},
		{"unsloth/Qwen3-0.6B-Q8_0.gguf", false},
		{"", false},

		// owner/repo/file shorthand has 2 slashes.
		{"unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf", true},

		// Schemes and HF host prefixes always parse.
		{"https://huggingface.co/owner/repo", true},
		{"http://huggingface.co/owner/repo/tree/main", true},
		{"hf.co/owner/repo", true},
		{"HF.CO/owner/repo", true},
		{"HUGGINGFACE.CO/owner/repo", true},
		{"huggingface.co/owner/repo/resolve/main/file.gguf", true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			if got := needsParse(tc.input); got != tc.want {
				t.Errorf("needsParse(%q) = %v, want %v", tc.input, got, tc.want)
			}
		})
	}
}

func TestModelsResolveSource_AcceptedInputForms(t *testing.T) {
	// Pre-seed the catalog so the resolver hits the cache and no HF
	// network call is needed. Each input form below should normalise to
	// the same cached canonical id and return the same Resolution.
	dir := t.TempDir()
	catalogDir := filepath.Join(dir, "catalog")
	if err := os.MkdirAll(catalogDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	rfile := filepath.Join(catalogDir, "catalog.yaml")

	cached := Catalog{
		Providers: []string{"unsloth"},
		Models: map[string]CatalogEntry{
			"unsloth/Qwen3-0.6B-Q8_0": {
				Provider: "unsloth",
				Family:   "Qwen3-0.6B-GGUF",
				Revision: "main",
				Files:    []string{"Qwen3-0.6B-Q8_0.gguf"},
			},
		},
	}
	data, _ := yaml.Marshal(cached)
	mustWriteFile(t, rfile, string(data))

	m, err := NewWithPaths(dir)
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"bare-id", "Qwen3-0.6B-Q8_0"},
		{"canonical-id", "unsloth/Qwen3-0.6B-Q8_0"},
		{"canonical-id-with-gguf", "unsloth/Qwen3-0.6B-Q8_0.gguf"},
		{"owner-repo-file", "unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf"},
		{"hf-co-shorthand", "hf.co/unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf"},
		{"resolve-url", "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/resolve/main/Qwen3-0.6B-Q8_0.gguf"},
		{"blob-url", "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/blob/main/Qwen3-0.6B-Q8_0.gguf"},
		{"trailing-whitespace", "  unsloth/Qwen3-0.6B-Q8_0  "},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res, err := m.ResolveSource(context.Background(), tc.input)
			if err != nil {
				t.Fatalf("ResolveSource(%q): %v", tc.input, err)
			}
			if res.CanonicalID != "unsloth/Qwen3-0.6B-Q8_0" {
				t.Errorf("CanonicalID = %q, want unsloth/Qwen3-0.6B-Q8_0", res.CanonicalID)
			}
			if !res.FromCache {
				t.Error("expected FromCache=true (input should normalise to cached canonical id)")
			}
			if len(res.RepoFiles) != 0 {
				t.Errorf("RepoFiles = %v, want empty for resolved input", res.RepoFiles)
			}
		})
	}
}

func TestModelsResolveSource_EmptyInput(t *testing.T) {
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	for _, in := range []string{"", "   ", "\t\n"} {
		_, err := m.ResolveSource(context.Background(), in)
		if err == nil {
			t.Errorf("ResolveSource(%q): expected error, got nil", in)
			continue
		}
		if !strings.Contains(err.Error(), "empty source") {
			t.Errorf("ResolveSource(%q): err = %v, want 'empty source'", in, err)
		}
	}
}

func TestModelsResolveSource_InvalidShorthand(t *testing.T) {
	// A shorthand that doesn't decompose into owner/repo (e.g. "owner//file")
	// must surface a clean parse error rather than reaching the resolver.
	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	_, err = m.ResolveSource(context.Background(), "https://huggingface.co/")
	if err == nil {
		t.Fatal("expected error for empty owner/repo URL")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("err = %v, want 'parse' substring", err)
	}
}

// TestModelsResolveSource_RepoOnlyReturnsRepoFiles exercises the input
// forms that identify a repository without pinning a file (tree URL,
// bare repo URL). Both must skip the resolver entirely and return a
// Resolution carrying RepoFiles for the BUI picker. listRepoGGUFsFn is
// swapped to a hermetic stub so the test does not hit huggingface.co.
func TestModelsResolveSource_RepoOnlyReturnsRepoFiles(t *testing.T) {
	want := []hf.RepoFile{
		{Filename: "Qwen3-0.6B-Q4_K_M.gguf", Size: 100},
		{Filename: "Qwen3-0.6B-Q8_0.gguf", Size: 200},
	}

	var gotOwner, gotRepo string
	var calls int

	prev := listRepoGGUFsFn
	listRepoGGUFsFn = func(_ context.Context, owner, repo string) ([]hf.RepoFile, error) {
		calls++
		gotOwner, gotRepo = owner, repo
		return want, nil
	}
	t.Cleanup(func() { listRepoGGUFsFn = prev })

	m, err := NewWithPaths(t.TempDir())
	if err != nil {
		t.Fatalf("NewWithPaths: %v", err)
	}

	tests := []struct {
		name  string
		input string
	}{
		{"tree-url", "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF/tree/main"},
		{"repo-url", "https://huggingface.co/unsloth/Qwen3-0.6B-GGUF"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls = 0

			res, err := m.ResolveSource(context.Background(), tc.input)
			if err != nil {
				t.Fatalf("ResolveSource(%q): %v", tc.input, err)
			}

			if calls != 1 {
				t.Errorf("listRepoGGUFsFn calls = %d, want 1", calls)
			}
			if gotOwner != "unsloth" || gotRepo != "Qwen3-0.6B-GGUF" {
				t.Errorf("listRepoGGUFsFn args = (%q, %q), want (unsloth, Qwen3-0.6B-GGUF)", gotOwner, gotRepo)
			}
			if res.Provider != "unsloth" || res.Family != "Qwen3-0.6B-GGUF" {
				t.Errorf("Resolution provider/family = %q/%q, want unsloth/Qwen3-0.6B-GGUF", res.Provider, res.Family)
			}
			if !reflect.DeepEqual(res.RepoFiles, want) {
				t.Errorf("RepoFiles = %v, want %v", res.RepoFiles, want)
			}
			if res.CanonicalID != "" || len(res.DownloadURLs) != 0 {
				t.Errorf("repo-only input must not produce a resolved download: canonical=%q urls=%v", res.CanonicalID, res.DownloadURLs)
			}
		})
	}
}

func TestSplitProviderRepoTag(t *testing.T) {
	tests := []struct {
		name                        string
		in                          string
		wantOK                      bool
		wantProv, wantRepo, wantTag string
	}{
		{
			name:     "valid",
			in:       "unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q4_K_XL",
			wantOK:   true,
			wantProv: "unsloth", wantRepo: "Qwen3.6-35B-A3B-GGUF", wantTag: "UD-Q4_K_XL",
		},
		{
			name:     "valid-simple-quant",
			in:       "ggml-org/embeddinggemma-300m-qat:Q8_0",
			wantOK:   true,
			wantProv: "ggml-org", wantRepo: "embeddinggemma-300m-qat", wantTag: "Q8_0",
		},
		{
			name:   "no-colon",
			in:     "unsloth/Qwen3-0.6B-Q8_0",
			wantOK: false,
		},
		{
			name:   "no-slash",
			in:     "Qwen3-GGUF:Q4_K_M",
			wantOK: false,
		},
		{
			name:   "two-slashes",
			in:     "unsloth/Qwen3-0.6B-GGUF/Qwen3-0.6B-Q8_0.gguf",
			wantOK: false,
		},
		{
			name:   "two-slashes-with-tag",
			in:     "unsloth/Qwen3-0.6B-GGUF/sub:Q8_0",
			wantOK: false,
		},
		{
			name:   "empty-tag",
			in:     "unsloth/Qwen3-GGUF:",
			wantOK: false,
		},
		{
			name:   "empty-repo",
			in:     "unsloth/:Q8_0",
			wantOK: false,
		},
		{
			name:   "empty-provider",
			in:     "/repo:Q8_0",
			wantOK: false,
		},
		{
			name:   "double-colon",
			in:     "unsloth/repo:Q8:0",
			wantOK: false,
		},
		{
			name:   "empty",
			in:     "",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider, repo, tag, ok := splitProviderRepoTag(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if provider != tt.wantProv || repo != tt.wantRepo || tag != tt.wantTag {
				t.Errorf("got (%q, %q, %q), want (%q, %q, %q)",
					provider, repo, tag, tt.wantProv, tt.wantRepo, tt.wantTag)
			}
		})
	}
}

func TestFileMatchesTag(t *testing.T) {
	tests := []struct {
		file, tag string
		want      bool
	}{
		{"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf", "UD-Q4_K_XL", true},
		{"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf", "Q4_K_XL", true},    // suffix still matches
		{"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf", "Q4_K_M", false},    // distinct quants
		{"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf", "ud-q4_k_xl", true}, // case-insensitive
		{"Qwen2-Audio-7B.Q8_0.gguf", "Q8_0", true},              // dot separator
		{"Qwen3-Q4_K_M-00001-of-00002.gguf", "Q4_K_M", true},    // split suffix stripped
		{"Qwen3-Q8_0.gguf", "", false},
		{"Qwen3-Q8_0.gguf", "9_0", false}, // partial without separator must not match
	}

	for _, tt := range tests {
		got := fileMatchesTag(tt.file, tt.tag)
		if got != tt.want {
			t.Errorf("fileMatchesTag(%q, %q) = %v, want %v", tt.file, tt.tag, got, tt.want)
		}
	}
}

func TestSelectFilesByTag(t *testing.T) {
	t.Run("exact-match", func(t *testing.T) {
		siblings := []string{
			"README.md",
			"Qwen3.6-35B-A3B-Q4_K_M.gguf",
			"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf",
			"mmproj-F16.gguf",
		}
		files, mmproj, ok := selectFilesByTag(siblings, "UD-Q4_K_XL")
		if !ok {
			t.Fatal("expected match")
		}
		if !reflect.DeepEqual(files, []string{"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"}) {
			t.Errorf("files = %v", files)
		}
		if mmproj != "mmproj-F16.gguf" {
			t.Errorf("mmproj = %q", mmproj)
		}
	})

	t.Run("prefers-ud-when-multiple", func(t *testing.T) {
		siblings := []string{
			"Qwen3-Q4_K_XL.gguf",
			"Qwen3-UD-Q4_K_XL.gguf",
		}
		files, _, ok := selectFilesByTag(siblings, "Q4_K_XL")
		if !ok {
			t.Fatal("expected match")
		}
		if !reflect.DeepEqual(files, []string{"Qwen3-UD-Q4_K_XL.gguf"}) {
			t.Errorf("files = %v, want [Qwen3-UD-Q4_K_XL.gguf]", files)
		}
	})

	t.Run("no-match", func(t *testing.T) {
		siblings := []string{
			"Qwen3-Q5_K_M.gguf",
			"Qwen3-Q8_0.gguf",
		}
		if _, _, ok := selectFilesByTag(siblings, "Q4_K_XL"); ok {
			t.Fatal("expected no match")
		}
	})

	t.Run("split-files", func(t *testing.T) {
		siblings := []string{
			"Llama-3.3-70B-Q8_0/Llama-3.3-70B-Q8_0-00001-of-00002.gguf",
			"Llama-3.3-70B-Q8_0/Llama-3.3-70B-Q8_0-00002-of-00002.gguf",
		}
		files, _, ok := selectFilesByTag(siblings, "Q8_0")
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
	})

	t.Run("dot-separator-mradermacher", func(t *testing.T) {
		siblings := []string{
			"Qwen2-Audio-7B.Q8_0.gguf",
			"Qwen2-Audio-7B.mmproj-f16.gguf",
		}
		files, mmproj, ok := selectFilesByTag(siblings, "Q8_0")
		if !ok {
			t.Fatal("expected match")
		}
		if !reflect.DeepEqual(files, []string{"Qwen2-Audio-7B.Q8_0.gguf"}) {
			t.Errorf("files = %v", files)
		}
		if mmproj != "Qwen2-Audio-7B.mmproj-f16.gguf" {
			t.Errorf("mmproj = %q", mmproj)
		}
	})

	t.Run("empty-tag", func(t *testing.T) {
		siblings := []string{"Qwen3-Q8_0.gguf"}
		if _, _, ok := selectFilesByTag(siblings, ""); ok {
			t.Fatal("expected no match for empty tag")
		}
	})
}

func TestResolver_TagForm_HFLookup(t *testing.T) {
	// Tag form should hit ModelMeta directly (no SearchModels), pick the
	// matching sibling, build correct download URLs, and persist the
	// resulting catalog entry under the canonical id derived from the
	// chosen file's basename.
	hfc := &fakeHF{
		metas: map[string][]string{
			"unsloth/Qwen3.6-35B-A3B-GGUF": {
				"Qwen3.6-35B-A3B-Q4_K_M.gguf",
				"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf",
				"mmproj-F16.gguf",
			},
		},
	}

	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers: []\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hfc)

	res, err := r.Resolve(context.Background(), "unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q4_K_XL")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	wantCanonical := "unsloth/Qwen3.6-35B-A3B-UD-Q4_K_XL"
	if res.CanonicalID != wantCanonical {
		t.Errorf("CanonicalID = %q, want %q", res.CanonicalID, wantCanonical)
	}
	if res.Provider != "unsloth" || res.Family != "Qwen3.6-35B-A3B-GGUF" {
		t.Errorf("Provider/Family = %q/%q", res.Provider, res.Family)
	}

	wantURL := "https://huggingface.co/unsloth/Qwen3.6-35B-A3B-GGUF/resolve/main/Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"
	if !reflect.DeepEqual(res.DownloadURLs, []string{wantURL}) {
		t.Errorf("DownloadURLs = %v\nwant [%s]", res.DownloadURLs, wantURL)
	}

	wantProj := "https://huggingface.co/unsloth/Qwen3.6-35B-A3B-GGUF/resolve/main/mmproj-F16.gguf"
	if res.DownloadProj != wantProj {
		t.Errorf("DownloadProj = %q\nwant %q", res.DownloadProj, wantProj)
	}

	// Ensure we did NOT call SearchModels — only ModelMeta.
	for _, c := range hfc.calls {
		if strings.HasPrefix(c, "search:") {
			t.Errorf("unexpected SearchModels call: %s", c)
		}
	}

	// And the entry was persisted under the canonical id.
	saved := loadResolved(t, rfile)
	if _, ok := saved.Models[wantCanonical]; !ok {
		t.Errorf("catalog missing %q; have %v", wantCanonical, mapKeys(saved.Models))
	}
}

func TestResolver_TagForm_PinsExplicitRepo(t *testing.T) {
	// Regression: when two repos under the same provider publish the
	// same quant basename (e.g. ".../Qwen3.6-35B-A3B-GGUF" and the
	// sibling ".../Qwen3.6-35B-A3B-MTP-GGUF" both ship
	// "Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf"), an "owner/repo/file" input
	// from the BUI Pull screen must land in the repo the user named.
	// The fix routes those inputs through the "provider/repo:tag" form
	// in ResolveSource so we hit ModelMeta on the explicit repo and
	// never reach SearchModels (which could pick the wrong sibling).
	hfc := &fakeHF{
		metas: map[string][]string{
			"unsloth/Qwen3.6-35B-A3B-GGUF": {
				"Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf",
			},
			"unsloth/Qwen3.6-35B-A3B-MTP-GGUF": {
				"Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf",
				"mmproj-F16.gguf",
			},
		},
		// If the resolver ever falls back to SearchModels, the MTP
		// repo would be returned first and the test would fail.
		search: map[string][]string{
			"unsloth|Qwen3.6-35B-A3B": {
				"unsloth/Qwen3.6-35B-A3B-MTP-GGUF",
				"unsloth/Qwen3.6-35B-A3B-GGUF",
			},
		},
	}

	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers: [unsloth]\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hfc)

	res, err := r.Resolve(context.Background(), "unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q8_K_XL")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	if res.Family != "Qwen3.6-35B-A3B-GGUF" {
		t.Errorf("Family = %q, want Qwen3.6-35B-A3B-GGUF (NOT the MTP sibling)", res.Family)
	}

	wantURL := "https://huggingface.co/unsloth/Qwen3.6-35B-A3B-GGUF/resolve/main/Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf"
	if !reflect.DeepEqual(res.DownloadURLs, []string{wantURL}) {
		t.Errorf("DownloadURLs = %v\nwant [%s]", res.DownloadURLs, wantURL)
	}

	// Must have hit ModelMeta on the explicit repo only — never
	// SearchModels, never the MTP repo.
	for _, c := range hfc.calls {
		if strings.HasPrefix(c, "search:") {
			t.Errorf("unexpected SearchModels call: %s", c)
		}
		if strings.Contains(c, "MTP") {
			t.Errorf("unexpected call against MTP sibling: %s", c)
		}
	}
}

func TestResolver_TagForm_CacheHit(t *testing.T) {
	// A pre-seeded entry whose family+tag match the request should serve
	// from cache without any HF call.
	hfc := &fakeHF{}
	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")

	cached := Catalog{
		Providers: []string{"unsloth"},
		Models: map[string]CatalogEntry{
			"unsloth/Qwen3.6-35B-A3B-UD-Q4_K_XL": {
				Provider:   "unsloth",
				Family:     "Qwen3.6-35B-A3B-GGUF",
				Revision:   "main",
				Files:      []string{"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"},
				MMProj:     "mmproj-Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf",
				MMProjOrig: "mmproj-F16.gguf",
			},
		},
	}
	data, _ := yaml.Marshal(cached)
	mustWriteFile(t, rfile, string(data))

	r := NewResolverWithClient(nil, rfile, hfc)

	res, err := r.Resolve(context.Background(), "unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q4_K_XL")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if !res.FromCache {
		t.Error("expected FromCache=true")
	}
	if len(hfc.calls) > 0 {
		t.Errorf("expected zero HF calls, got %v", hfc.calls)
	}

	wantURL := "https://huggingface.co/unsloth/Qwen3.6-35B-A3B-GGUF/resolve/main/Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"
	if !reflect.DeepEqual(res.DownloadURLs, []string{wantURL}) {
		t.Errorf("DownloadURLs = %v", res.DownloadURLs)
	}
}

func TestResolver_TagForm_TagNotFound(t *testing.T) {
	hfc := &fakeHF{
		metas: map[string][]string{
			"unsloth/Qwen3-GGUF": {
				"Qwen3-Q4_K_M.gguf",
				"Qwen3-Q8_0.gguf",
			},
		},
	}
	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers: []\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hfc)

	_, err := r.Resolve(context.Background(), "unsloth/Qwen3-GGUF:UD-Q4_K_XL")
	if err == nil {
		t.Fatal("expected error for missing tag")
	}
	if !strings.Contains(err.Error(), "tag") {
		t.Errorf("err = %v, want a 'tag' message", err)
	}
}

func TestResolver_TagForm_RepoNotFound(t *testing.T) {
	hfc := &fakeHF{
		missing: map[string]bool{
			"unsloth/Bogus-GGUF": true,
		},
	}
	dir := t.TempDir()
	rfile := filepath.Join(dir, "catalog.yaml")
	mustWriteFile(t, rfile, "providers: []\nmodels: {}\n")

	r := NewResolverWithClient(nil, rfile, hfc)

	_, err := r.Resolve(context.Background(), "unsloth/Bogus-GGUF:Q8_0")
	if err == nil {
		t.Fatal("expected error for missing repo")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("err = %v, want 'not found'", err)
	}
}

func TestResolver_AllInputForms_ProduceSameDownloadURL(t *testing.T) {
	// Every accepted input shape that points at a specific file must
	// produce the same set of download URLs as the canonical id form.
	const wantURL = "https://huggingface.co/unsloth/Qwen3.6-35B-A3B-GGUF/resolve/main/Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"

	cached := Catalog{
		Providers: []string{"unsloth"},
		Models: map[string]CatalogEntry{
			"unsloth/Qwen3.6-35B-A3B-UD-Q4_K_XL": {
				Provider: "unsloth",
				Family:   "Qwen3.6-35B-A3B-GGUF",
				Revision: "main",
				Files:    []string{"Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"},
			},
		},
	}
	data, _ := yaml.Marshal(cached)

	tests := []struct {
		name  string
		input string
	}{
		{"canonical-id", "unsloth/Qwen3.6-35B-A3B-UD-Q4_K_XL"},
		{"canonical-id-with-gguf", "unsloth/Qwen3.6-35B-A3B-UD-Q4_K_XL.gguf"},
		{"tag-form", "unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q4_K_XL"},
		{"tag-form-with-gguf", "unsloth/Qwen3.6-35B-A3B-GGUF:UD-Q4_K_XL.gguf"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			rfile := filepath.Join(dir, "catalog.yaml")
			mustWriteFile(t, rfile, string(data))

			r := NewResolverWithClient(nil, rfile, &fakeHF{})

			res, err := r.Resolve(context.Background(), tc.input)
			if err != nil {
				t.Fatalf("Resolve(%q): %v", tc.input, err)
			}
			if !reflect.DeepEqual(res.DownloadURLs, []string{wantURL}) {
				t.Errorf("DownloadURLs = %v\nwant [%s]", res.DownloadURLs, wantURL)
			}
		})
	}
}

// =============================================================================

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func mapKeys(m map[string]CatalogEntry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
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
