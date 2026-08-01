package model

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"go.yaml.in/yaml/v2"
)

// =============================================================================
// Silent test-coverage holes in the MTP integration suites.
//
// Two suites under sdk/kronk/tests/ resolve a model id that is absent from the
// catalog. resolveModel discards the error, TestMain reads the zero Path as
// "model not downloaded", and the suite exits 0 — green, silent, zero tests run.
// =============================================================================

// =============================================================================
// Silently skipping MTP test suites.

// mtpCatalogModelIDs returns the set of model ids declared in the shipped
// catalog, keyed both by the full "provider/name" form and by the bare name,
// which is what testlib passes to Models.FullPath (see fullPathLookupKeys /
// sdk/tools/models/retrieve.go:198).
func mtpCatalogModelIDs(t *testing.T) (full map[string]map[string]any, bare map[string]string) {
	t.Helper()

	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "tools", "defaults", "yaml", "catalog.yaml")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read catalog: %v", err)
	}

	var catalog struct {
		Models map[string]map[string]any `yaml:"models"`
	}

	if err := yaml.Unmarshal(data, &catalog); err != nil {
		t.Fatalf("parse catalog %s: %v", path, err)
	}

	if len(catalog.Models) == 0 {
		t.Fatalf("catalog %s declares no models", path)
	}

	bare = make(map[string]string, len(catalog.Models))
	for id := range catalog.Models {
		if idx := strings.LastIndex(id, "/"); idx >= 0 {
			bare[id[idx+1:]] = id
			continue
		}
		bare[id] = id
	}

	return catalog.Models, bare
}

// resolveModelCallRE matches testlib.Setup's catalog lookups, e.g.
//
//	resolveModel(mdls, "mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL", &MPMTP)
var resolveModelCallRE = regexp.MustCompile(`resolveModel\(mdls,\s*"([^"]+)",\s*&(\w+)\)`)

// TestTestlibResolvesOnlyCatalogModelIDs pins a silent test-coverage hole: the
// integration suites resolve model ids that the shipped catalog does not
// contain, and the resolution failure is swallowed.
//
// FINDING
// sdk/kronk/tests/testlib/testlib.go:96-106 resolves every suite's model, and
// sdk/kronk/tests/testlib/testlib.go:110-116 (resolveModel) DISCARDS the error:
//
//	func resolveModel(mdls *models.Models, name string, mp *models.Path) {
//	    if dp, err := mdls.FullPath(name); err == nil {   // err dropped
//	        *mp = dp
//	    }
//	}
//
// A bad id therefore leaves the models.Path zero-valued, and each suite's
// TestMain reads that as "model not downloaded" and calls os.Exit(0) — a green,
// silent, zero-test run. sdk/kronk/tests/testlib/testlib.go:97 asks for
// "mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL", which is not in
// sdk/tools/defaults/yaml/catalog.yaml; the only MTP-capable entry is
// unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL (catalog.yaml:321). An id absent from
// the catalog can never enter the on-disk download index that
// Models.FullPath consults (sdk/tools/models/retrieve.go:198-207), so
// testlib.MPMTP is permanently empty and sdk/kronk/tests/mtp/main_test.go:20
// exits 0 every time.
//
// LLAMA.CPP REFERENCE
// Not a llama.cpp divergence — this is what let the MTP divergences from
// llama.cpp's reference speculative path (.extras/llama.cpp/common/speculative.cpp,
// .extras/llama.cpp/tools/server/server-context.cpp) ship unnoticed: the suite
// that was supposed to catch them has never executed a single test.
//
// FAILURE SCENARIO
// CI reports the mtp package as passing. It printed
// "model mtp-Qwen3.6-35B-A3B-UD-Q2_K_XL not downloaded, skipping mtp tests"
// and ran nothing, so every MTP regression — including the ones behind the
// user's contradictions and abandoned tasks — is unguarded.
func TestTestlibResolvesOnlyCatalogModelIDs(t *testing.T) {
	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "tests", "testlib", "testlib.go")

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	matches := resolveModelCallRE.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatalf("no resolveModel(mdls, ...) calls found in %s; update this assertion", path)
	}

	_, bare := mtpCatalogModelIDs(t)

	for _, m := range matches {
		id, target := m[1], m[2]

		if _, ok := bare[id]; !ok {
			t.Errorf("testlib.go resolves model id %q into %s, but it is not in sdk/tools/defaults/yaml/catalog.yaml: it can never be downloaded, resolveModel swallows the error (testlib.go:110-116), and the suite gated on %s silently exits 0",
				id, target, target)
		}
	}
}

// TestGemma4MTPSuiteTargetShipsMTPSidecar pins the same silent-skip defect in
// the second MTP suite, which fails for a different reason: the id resolves,
// but the catalog entry carries no MTP companion file.
//
// FINDING
// sdk/kronk/tests/gemma4mtp/main_test.go:19-22 exits 0 unless
// testlib.MPMoEVision.MTPFile is non-empty. MPMoEVision is
// gemma-4-26B-A4B-it-UD-Q4_K_M (sdk/kronk/tests/testlib/testlib.go:89), and
// models.Path.MTPFile is populated only from the catalog entry's `mtp:` key
// (sdk/tools/models/catalog.go:787-789, sdk/tools/models/download.go:520).
// catalog.yaml declares `mtp:` only on unsloth/gemma-4-26B-A4B-it-UD-Q8_K_XL
// (catalog.yaml:352); the Q4_K_M entry (catalog.yaml:127-145) has none. So
// MTPFile is always "" and the whole gemma4mtp suite — the only coverage for
// the shared-KV MTP drafter path that Config.MTPDrafterFile selects
// (sdk/kronk/model/draft_mtp.go:438-458) — has never run.
//
// LLAMA.CPP REFERENCE
// The shared-KV drafter mirrors upstream's ctx_dft-shares-target-memory MTP
// implementation (.extras/llama.cpp/common/speculative.cpp:1286,
// COMMON_SPECULATIVE_TYPE_DRAFT_MTP). Upstream downloads that sidecar only on
// explicit request (.extras/llama.cpp/common/arg.cpp:388), which is why the
// catalog has to declare it per quant — and why the missing declaration on the
// Q4_K_M quant silently disables the suite instead of failing it.
//
// FAILURE SCENARIO
// A change to loadDraftModelMTPShared or to the Pass 2A/2B split lands with a
// green gemma4mtp package that executed zero tests. Either point the suite at
// the Q8_K_XL quant that actually ships the sidecar, or add the `mtp:` entry to
// the Q4_K_M catalog record.
func TestGemma4MTPSuiteTargetShipsMTPSidecar(t *testing.T) {
	models, bare := mtpCatalogModelIDs(t)

	const target = "gemma-4-26B-A4B-it-UD-Q4_K_M"

	full, ok := bare[target]
	if !ok {
		t.Fatalf("catalog has no entry for %q; testlib.go:89 resolves it for MPMoEVision", target)
	}

	if _, ok := models[full]["mtp"]; !ok {
		t.Errorf("catalog entry %q declares no mtp: sidecar, so testlib.MPMoEVision.MTPFile is always empty and sdk/kronk/tests/gemma4mtp/main_test.go:19 exits 0 without running a single test", full)
	}
}
