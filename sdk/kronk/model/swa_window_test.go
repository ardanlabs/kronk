package model

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Sliding-window attention (SWA) runtime semantics.
//
// GROUND FACT for the model this audit targets
// (unsloth/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL, general.architecture = "qwen35moe"):
//
//	n_swa == 0 and swa_type == LLAMA_SWA_TYPE_NONE.
//
// Evidence:
//   - The GGUF carries NO "qwen35moe.attention.sliding_window" key (nor any
//     "*.sliding_window*" key at all). It carries
//     qwen35moe.full_attention_interval = 4 plus ssm.* keys instead: the
//     non-full-attention layers are RECURRENT (gated DeltaNet), not windowed.
//   - .extras/llama.cpp/src/models/qwen35moe.cpp:4-38 (load_arch_hparams)
//     never touches hparams.n_swa or hparams.swa_type, and no generic loader
//     reads a sliding-window key for this arch. The defaults therefore stand:
//     .extras/llama.cpp/src/llama-hparams.h:143 (swa_type = LLAMA_SWA_TYPE_NONE)
//     and :145 (n_swa = 0).
//   - Consequently .extras/llama.cpp/src/llama-model.cpp:2185-2206 selects plain
//     llama_memory_hybrid (attn + recurrent), NOT llama_memory_hybrid_iswa —
//     there is no SWA sub-cache in this model's memory module at all.
//
// So SWA cannot be the mechanism behind "the model drops the task mid-way" for
// THIS model. The tests below pin the two SWA defects that do exist in Kronk;
// both are latent for qwen35moe and bite the genuinely windowed architectures
// (gemma2/3/4, cohere2, exaone4, ...).

// =============================================================================

// TestCalculateVRAMDiagResolvesEffectiveSWAFull pins a wrong-default bug in the
// SWA branch of Kronk's in-process KV-cache estimator.
//
// FINDING
// Config.SWAFull() (sdk/kronk/model/config.go:448) resolves a nil PtrSWAFull to
// FALSE, and its own doc comment (config.go:445-447) says so explicitly:
// "reports whether a full-size sliding-window attention cache was EXPLICITLY
// REQUESTED. Callers that need the effective value must check PtrSWAFull first
// because nil leaves the choice to llama.cpp."
//
// calculateVRAMDiag (sdk/kronk/model/model.go:1331) is exactly such a caller and
// does not honour the contract:
//
//	SWAFull: cfg.SWAFull(),
//
// The two peer call sites that resolve the SAME question both map nil to TRUE:
//   - sdk/kronk/pool/llama.go:315-319 (effectiveSWAFull), used for the
//     admission/loader estimate at pool/llama.go:289.
//   - sdk/tools/models/analyze.go:271 (`cfg.PtrSWAFull == nil || *cfg.PtrSWAFull`).
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/src/llama-context.cpp:3512 — llama_context_default_params
//     sets `swa_full = true`. Kronk starts from llama.ContextDefaultParams()
//     (config.go:842) and only overwrites SwaFull when PtrSWAFull != nil
//     (config.go:961-967), so nil genuinely means swa_full == TRUE at runtime.
//   - .extras/llama.cpp/src/llama-kv-cache-iswa.cpp:69-80 — the allocator:
//     size_swa = PAD(min(size_base, n_swa*(unified ? n_seq_max : 1) + n_ubatch), 256)
//     and `if (swa_full) { size_swa = size_base; }`.
//     sdk/kronk/gguf/kvcache.go:83-89 is a faithful port of that formula, so the
//     estimate is only as correct as the SWAFull flag handed to it.
//
// CONCRETE FAILURE SCENARIO
// A gemma3-class model (n_swa = 1024, 262144-token context) is loaded with
// swa-full unset. The live llama_context allocates size_swa == size_base for
// every SWA layer, but calculateVRAMDiag computes the compact
// n_swa + n_ubatch size, so ModelInfo.SlotMemory / VRAMTotal — the values
// pool/llama.go:307-308 falls back to and the loader displays — under-report the
// SWA layers' KV by orders of magnitude, while the resman estimate one call away
// (pool/llama.go:289) reports the correct larger number for the same config.
//
// The test accepts EITHER fix: making Config.SWAFull() nil-aware, or resolving
// the default at the model.go:1331 call site.
func TestCalculateVRAMDiagResolvesEffectiveSWAFull(t *testing.T) {
	// llamaCppDefaultSwaFull is llama_context_default_params().swa_full in
	// llama.cpp b10211 (.extras/llama.cpp/src/llama-context.cpp:3512). Kronk
	// never overrides it when PtrSWAFull is nil (config.go:961-967), so this
	// is the effective runtime value for an unset config.
	const llamaCppDefaultSwaFull = true

	// Sanity: the accessor must still report explicit values verbatim.
	yes, no := true, false
	if got := (Config{PtrSWAFull: &yes}).SWAFull(); !got {
		t.Fatalf("Config{PtrSWAFull: &true}.SWAFull() = %v, want true", got)
	}
	if got := (Config{PtrSWAFull: &no}).SWAFull(); got {
		t.Fatalf("Config{PtrSWAFull: &false}.SWAFull() = %v, want false", got)
	}

	// Fix A: the accessor itself became nil-aware. Nothing else to check.
	if (Config{}).SWAFull() == llamaCppDefaultSwaFull {
		return
	}

	// Fix B: calculateVRAMDiag resolves the default itself. Locate the
	// SWAFull field of the vram.Config literal it builds and require the
	// expression to consult PtrSWAFull.
	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "model", "model.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)
	fn := findKronkFunc(t, file, path, "calculateVRAMDiag")

	expr, pos := swaFullFieldValue(fn)
	if expr == nil {
		t.Fatalf("%s: calculateVRAMDiag has no `SWAFull:` field in its vram.Config literal; this test pins a construction that changed — re-point it",
			srcPos(fset, root, fn.Pos()))
	}

	text := swaSourceText(t, fset, path, expr)
	if strings.Contains(text, "PtrSWAFull") {
		return
	}

	t.Errorf(`calculateVRAMDiag sizes the SWA KV cache from a value that resolves an unset swa-full to false.

  site:     %s
            SWAFull: %s
  bug:      Config.SWAFull() (sdk/kronk/model/config.go:448) returns
            boolOr(cfg.PtrSWAFull, false) and its own doc comment
            (config.go:445-447) warns that callers needing the EFFECTIVE value
            must inspect PtrSWAFull, because nil leaves the choice to llama.cpp.
  truth:    llama_context_default_params() sets swa_full = true
            (.extras/llama.cpp/src/llama-context.cpp:3512) and Kronk only
            overwrites ctxParams.SwaFull when PtrSWAFull != nil
            (sdk/kronk/model/config.go:961-967). Unset therefore means
            swa_full == true in the live context.
  effect:   sdk/kronk/gguf/kvcache.go:83-89 ports
            .extras/llama.cpp/src/llama-kv-cache-iswa.cpp:69-80 — with
            SWAFull == false it shrinks the SWA layers to
            PAD(min(baseCells, n_swa*seq + n_ubatch), 256) instead of the
            full-size cache llama.cpp actually allocates. ModelInfo.SlotMemory
            and VRAMTotal (the loader fallback at
            sdk/kronk/pool/llama.go:307-308) then under-report SWA-layer KV for
            every windowed model whose swa-full is unset.
  peers:    sdk/kronk/pool/llama.go:315-319 (effectiveSWAFull) and
            sdk/tools/models/analyze.go:271 both resolve nil to TRUE for the
            same question, so the two estimates disagree for one config.
  fix:      either make Config.SWAFull() nil-aware, or resolve the default here
            the way pool/llama.go:315-319 does.
  scope:    latent for the audited qwen35moe target (n_swa == 0, see the file
            header) — it bites gemma2/3/4, cohere2, exaone4 and friends.`,
		srcPos(fset, root, pos), text)
}

// swaFullFieldValue returns the value expression of the first `SWAFull:` element
// of any composite literal inside fn, plus the position of the element.
func swaFullFieldValue(fn *ast.FuncDecl) (ast.Expr, token.Pos) {
	var found ast.Expr
	var at token.Pos

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if found != nil {
			return false
		}

		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}

		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "SWAFull" {
				continue
			}
			found = kv.Value
			at = kv.Pos()

			return false
		}

		return true
	})

	return found, at
}

// swaSourceText returns the verbatim source text of node as it appears in path.
func swaSourceText(t *testing.T, fset *token.FileSet, path string, node ast.Node) string {
	t.Helper()

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	lo := fset.Position(node.Pos()).Offset
	hi := fset.Position(node.End()).Offset
	if lo < 0 || hi > len(src) || lo > hi {
		t.Fatalf("%s: node offsets [%d,%d) outside the %d-byte file", path, lo, hi, len(src))
	}

	return string(src[lo:hi])
}

// =============================================================================

// TestKronkSurfacesEffectiveNSWA pins an observability gap: nothing Kronk emits
// lets an operator tell whether the loaded model uses sliding-window attention,
// nor how large the window is.
//
// FINDING
// The only SWA value Kronk prints is the REQUESTED context param:
// sdk/kronk/model/model.go:637-643 logs "SwaFull[%d]" from ctxParams.SwaFull.
// That is the flag Kronk asked for, not the model's window, and it is emitted
// even for models with no SWA layers at all. Nothing anywhere in the SDK calls
// llama.ModelNSWA (.extras/yzma/pkg/llama/model.go:534, binding
// llama_model_n_swa), so the effective n_swa is never observable.
//
// The usual escape hatch is closed too: llama.cpp's own load banner prints
// "n_swa = %u" (.extras/llama.cpp/src/llama-model.cpp:1785), but
// sdk/kronk/init.go:134-140 defaults an out-of-range log level to LogSilent and
// installs llama.LogSet(llama.LogSilent()), so that line never reaches an
// operator in the default configuration.
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/src/llama-model.cpp:1785 — the "n_swa" load line.
//   - .extras/llama.cpp/src/llama-model.cpp:2443-2449 — llama_model_n_swa, the
//     public accessor, which upstream's own server calls at
//     tools/server/server-context.cpp:1280-1287 precisely to decide whether SWA
//     handling (and swa_full) applies before configuring prompt reuse and
//     context checkpoints.
//
// CONCRETE FAILURE SCENARIO
// An operator debugging "the model forgets what it said earlier" cannot
// distinguish a genuinely windowed model running a compact SWA cache from a
// non-SWA model, because both log the identical "SwaFull[1]" line and llama.cpp
// is muted. Diagnosing this audit's target required dumping the GGUF by hand and
// reading llama.cpp's per-arch hparams loader.
//
// FIX: read llama.ModelNSWA(mdl) at load and log it (0 => no SWA layers)
// alongside the resolved swa_full, mirroring
// .extras/llama.cpp/src/llama-model.cpp:1785.
func TestKronkSurfacesEffectiveNSWA(t *testing.T) {
	root := kronkRepoRoot(t)
	sdk := filepath.Join(root, "sdk")

	var sites []string
	err := filepath.WalkDir(sdk, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(src), "ModelNSWA") {
			return nil
		}

		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			rel = path
		}
		sites = append(sites, rel)

		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", sdk, err)
	}

	if len(sites) > 0 {
		t.Logf("llama.ModelNSWA referenced from: %s", strings.Join(sites, ", "))
		return
	}

	t.Error(strings.TrimSpace(fmt.Sprintf(`
llama.ModelNSWA is never called anywhere under sdk/ — the effective n_swa is not observable.

  bug:      Kronk logs only the REQUESTED context param
            (sdk/kronk/model/model.go:637-643, "SwaFull[%%d]" from
            ctxParams.SwaFull). It never reports the model's window, so an
            operator cannot tell a windowed model running a compact SWA cache
            from a model with no SWA layers.
  muted:    llama.cpp prints "n_swa = %%u" at load
            (.extras/llama.cpp/src/llama-model.cpp:1785), but
            sdk/kronk/init.go:134-140 installs llama.LogSet(llama.LogSilent())
            for the default log level, so that banner is discarded.
  binding:  yzma already exports it: llama.ModelNSWA
            (.extras/yzma/pkg/llama/model.go:534 -> llama_model_n_swa,
            .extras/llama.cpp/src/llama-model.cpp:2443-2449).
  upstream: llama-server calls llama_model_n_swa before configuring prompt
            reuse / context checkpoints and warns when swa_full is meaningless
            for the model (tools/server/server-context.cpp:1280-1287).
  fix:      log llama.ModelNSWA(mdl) at model load (0 => no SWA layers) next to
            the resolved swa_full.
  note:     for the audited qwen35moe target n_swa == 0, so this line would have
            immediately ruled SWA out; see this file's header.`)))
}
