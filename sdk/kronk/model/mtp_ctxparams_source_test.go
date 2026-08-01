package model

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

// =============================================================================
// Shared source-analysis helpers.
//
// The three regressions pinned in this file all live inside functions that
// cannot be exercised without a loaded llama shared library AND a loaded GGUF:
//
//   - modelCtxParams (sdk/kronk/model/config.go:841) opens with
//     llama.ContextDefaultParams(), a raw FFI call with no nil-handle guard
//     (yzma .extras/yzma/pkg/llama/context.go:347-351). Without a prior
//     dlopen it panics with "invalid memory address or nil pointer
//     dereference". Even with the library loaded, the MTP branch is gated on
//     mtpNextNLayers(mdl) > 0 && MTPAvailable(), which needs a real model
//     handle plus the pre-norm symbols.
//   - captureTargetSpecSnapshot / restoreTargetSpecSnapshot need a live
//     llama.Context with populated KV.
//   - loadDraftModelMTP / loadDraftModelMTPShared build their llama.ContextParams
//     inline and hand them straight to llama.InitFromModel, so the params are
//     never observable from the outside.
//
// Loading the 39 GB MTP target is out of scope for a unit test, so these are
// AST assertions over the checked-in source instead. They locate everything by
// declaration name, never by line number, so they survive unrelated edits.

// kronkRepoRoot returns the absolute path of the kronk repository root.
func kronkRepoRoot(t *testing.T) string {
	t.Helper()

	if _, thisFile, _, ok := runtime.Caller(0); ok {
		root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			return root
		}
	}

	if ws := os.Getenv("GITHUB_WORKSPACE"); ws != "" {
		if _, err := os.Stat(filepath.Join(ws, "go.mod")); err == nil {
			return ws
		}
	}

	t.Fatal("cannot locate the kronk repo root; set GITHUB_WORKSPACE")

	return ""
}

// parseKronkSource parses path with comments attached.
func parseKronkSource(t *testing.T, fset *token.FileSet, path string) *ast.File {
	t.Helper()

	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	return f
}

// findKronkFunc returns the declaration named name, failing the test when the
// declaration has been renamed out from under the assertion.
func findKronkFunc(t *testing.T, f *ast.File, path string, name string) *ast.FuncDecl {
	t.Helper()

	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if ok && fd.Name.Name == name {
			return fd
		}
	}

	t.Fatalf("func %s not found in %s: the test pins a declaration that no longer exists; re-point it", name, path)

	return nil
}

// commentClaimPos returns the position of the individual comment line inside
// group that carries want, so failures point at the sentence rather than at the
// top of a long comment block.
func commentClaimPos(group *ast.CommentGroup, want string) token.Pos {
	for _, c := range group.List {
		if strings.Contains(c.Text, want) {
			return c.Pos()
		}
	}

	return group.Pos()
}

// srcPos renders a token.Pos as a repo-relative "file:line".
func srcPos(fset *token.FileSet, root string, p token.Pos) string {
	pos := fset.Position(p)

	rel, err := filepath.Rel(root, pos.Filename)
	if err != nil {
		rel = pos.Filename
	}

	return fmt.Sprintf("%s:%d", rel, pos.Line)
}

// nonTestGoFiles lists the non-test .go files in dir.
func nonTestGoFiles(t *testing.T, dir string) []string {
	t.Helper()

	all, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob %s: %v", dir, err)
	}

	var out []string
	for _, p := range all {
		if !strings.HasSuffix(p, "_test.go") {
			out = append(out, p)
		}
	}

	if len(out) == 0 {
		t.Fatalf("no non-test go files under %s", dir)
	}

	return out
}

// fieldWriteSites returns every repo-relative "file:line" in dir's non-test
// sources that writes a struct field named field, either as `x.Field = ...`
// or as a `Field:` element of a composite literal.
func fieldWriteSites(t *testing.T, root string, dir string, field string) []string {
	t.Helper()

	fset := token.NewFileSet()

	var sites []string
	for _, path := range nonTestGoFiles(t, dir) {
		f := parseKronkSource(t, fset, path)

		ast.Inspect(f, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range node.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == field {
						sites = append(sites, srcPos(fset, root, sel.Pos()))
					}
				}

			case *ast.CompositeLit:
				for _, elt := range node.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if key, ok := kv.Key.(*ast.Ident); ok && key.Name == field {
						sites = append(sites, srcPos(fset, root, kv.Pos()))
					}
				}
			}

			return true
		})
	}

	return sites
}

// =============================================================================

// TestModelCtxParamsAssignsNRsSeq pins findings2 §4: kronk never assigns
// llama.ContextParams.NRsSeq, so llama.cpp's bounded partial recurrent-state
// rollback is permanently disabled on hybrid MTP targets.
//
// llama.cpp b10211's llm_arch_supports_rs_rollback
// (.extras/llama.cpp/src/llama-arch.cpp:989-997) returns true for
// LLM_ARCH_QWEN35 and LLM_ARCH_QWEN35MOE. When n_rs_seq >= n_draft,
// llama_memory_seq_rm performs the partial recurrent rollback natively and the
// expensive per-round state snapshot in batchgen_speculative.go is unnecessary.
// Upstream derives it from the draft width
// (.extras/llama.cpp/common/common.cpp:1633, common/common.h:386-393).
//
// yzma already exposes the field (.extras/yzma/pkg/llama/llama.go:340) and a
// read-back accessor llama.NRsSeq (context.go:550). Kronk's only reference is
// the debug log at sdk/kronk/model/model.go:640, which prints the REQUESTED
// value — permanently 0 — so nothing in the logs reveals the miss.
//
// BUG PINNED: sdk/kronk/model/config.go:841 (modelCtxParams) never sets
// ctxParams.NRsSeq. It should be set to at least the MTP draft width
// (mtpNDraft(cfg)) whenever the target is an MTP-capable hybrid.
func TestModelCtxParamsAssignsNRsSeq(t *testing.T) {
	root := kronkRepoRoot(t)
	dir := filepath.Join(root, "sdk", "kronk", "model")

	sites := fieldWriteSites(t, root, dir, "NRsSeq")
	if len(sites) == 0 {
		t.Errorf(`llama.ContextParams.NRsSeq is never assigned anywhere in sdk/kronk/model/.

  bug:      findings2 §4 — partial recurrent-state rollback is disabled.
  pinned:   sdk/kronk/model/config.go:841 (modelCtxParams) builds the target
            llama.ContextParams and returns without touching NRsSeq, so
            llama.cpp receives n_rs_seq == 0 for every model.
  effect:   llm_arch_supports_rs_rollback is true for QWEN35 / QWEN35MOE
            (.extras/llama.cpp/src/llama-arch.cpp:989-997), but with
            n_rs_seq == 0 llama_memory_recurrent::seq_rm refuses every
            mid-sequence range, so llama_memory_hybrid::seq_rm returns false
            having mutated nothing (llama-memory-hybrid.cpp:143-150). Kronk
            then falls back to a full per-seq state snapshot every
            speculative round (batchgen_speculative.go:784,797,823).
  expected: modelCtxParams sets ctxParams.NRsSeq >= the MTP draft width
            (mtpNDraft(cfg)) for MTP-capable targets, mirroring upstream
            .extras/llama.cpp/common/common.cpp:1633.
  note:     sdk/kronk/model/model.go:640 logs ctxParams.NRsSeq — the REQUESTED
            value, always 0. Logging llama.NRsSeq(lctx) (yzma context.go:550)
            instead would report what llama.cpp actually honoured.`)
	}

	// Once the assignment lands this guards against it being logged-only.
	for _, s := range sites {
		t.Logf("NRsSeq write site: %s", s)
	}
}

// TestSpecSnapshotUsesPartialOnlyStateExt pins findings2 §4: the speculative
// snapshot/restore path saves and reloads the FULL per-sequence state —
// including the entire attention KV — on every speculative round.
//
// llama_memory_hybrid::state_write / state_read
// (.extras/llama.cpp/src/llama-memory-hybrid.cpp:190-202) skip the attention
// KV entirely when LLAMA_STATE_SEQ_FLAGS_PARTIAL_ONLY (value 1,
// .extras/llama.cpp/include/llama.h:898) is set, which is exactly what is
// wanted for a speculative checkpoint: only the recurrent state cannot be
// trimmed per-position. Upstream's server passes the flag for every
// speculative checkpoint (tools/server/server-context.cpp:2381-2382, 3042,
// 3061, 3073, 3888-3891).
//
// yzma already binds the flags-accepting variants —
// StateSeqGetSizeExt / StateSeqGetDataExt / StateSeqSetDataExt
// (.extras/yzma/pkg/llama/state.go:343,353,368) — so this is a three-line
// change, not FFI work.
//
// BUG PINNED: sdk/kronk/model/batchgen_speculative.go:784, 797, 823 call the
// non-_ext forms (flags implicitly 0).
func TestSpecSnapshotUsesPartialOnlyStateExt(t *testing.T) {
	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "model", "batchgen_speculative.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)

	// Non-_ext call -> the _ext form that must replace it.
	wantExt := map[string]string{
		"StateSeqGetSize": "StateSeqGetSizeExt",
		"StateSeqGetData": "StateSeqGetDataExt",
		"StateSeqSetData": "StateSeqSetDataExt",
	}

	funcs := []struct {
		name    string
		require []string
	}{
		{"captureTargetSpecSnapshot", []string{"StateSeqGetSize", "StateSeqGetData"}},
		{"restoreTargetSpecSnapshot", []string{"StateSeqSetData"}},
	}

	for _, fn := range funcs {
		fd := findKronkFunc(t, file, path, fn.name)

		seen := make(map[string]bool)

		ast.Inspect(fd.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}

			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			seen[sel.Sel.Name] = true

			ext, bad := wantExt[sel.Sel.Name]
			if !bad {
				return true
			}

			t.Errorf(`%s: %s calls llama.%s (flags implicitly 0) instead of llama.%s(..., 1).

  bug:      findings2 §4 — every speculative round snapshots the full per-seq
            state, attention KV included.
  fix:      llama.%s(..., uint32(1)) — LLAMA_STATE_SEQ_FLAGS_PARTIAL_ONLY,
            .extras/llama.cpp/include/llama.h:898. llama_memory_hybrid then
            skips the attention KV (llama-memory-hybrid.cpp:190-202).
  binding:  yzma already exports %s (.extras/yzma/pkg/llama/state.go).`,
				srcPos(fset, root, sel.Pos()), fn.name, sel.Sel.Name, ext, ext, ext)

			return true
		})

		// The _ext form must actually be present, so that deleting the
		// checkpoint altogether does not make this test pass. Only report it
		// when the non-_ext call is absent too — otherwise the violation above
		// already says everything.
		for _, base := range fn.require {
			ext := wantExt[base]
			if !seen[ext] && !seen[base] {
				t.Errorf("%s: %s calls neither llama.%s nor llama.%s; the PARTIAL_ONLY checkpoint (findings2 §4) is not implemented",
					srcPos(fset, root, fd.Pos()), fn.name, base, ext)
			}
		}
	}
}

// TestLoadDraftModelMTPCopiesTargetCtxParams pins findings2 §6f:
// loadDraftModelMTP (the own-KV Qwen embedded-MTP path) forwards eleven fields
// from the target's llama.ContextParams but silently drops KVUnified and
// SwaFull.
//
// The shared-KV loader loadDraftModelMTPShared copies both and documents why at
// sdk/kronk/model/draft_mtp.go:330-337: "They MUST match the target: ... a
// stream-layout mismatch ... makes llama_kv_cache::get_k compute a 4-D view
// that overruns the shared tensor".
//
// The own-KV path is not immune. modelCtxParams sets ctxParams.KVUnified = 1
// whenever nSeqMax > 1 (sdk/kronk/model/config.go:936-939) and
// NCtx = ContextWindow * nSeqMax (config.go:862). So at nSeqMax == 2 the target
// runs unified with n_ctx_seq == 2*window while the MTP draft context inherits
// the same NCtx but runs non-unified with n_stream == 2, giving
// n_ctx_seq == window: a different cache topology and half the per-sequence
// draft capacity the target has.
//
// Both loaders build their params inline and pass them straight to
// llama.InitFromModel, so the values are unobservable without a loaded model.
// This compares the two functions' copy sets instead, which also catches future
// drift in either direction.
//
// BUG PINNED: sdk/kronk/model/draft_mtp.go:86-98 (loadDraftModelMTP).
func TestLoadDraftModelMTPCopiesTargetCtxParams(t *testing.T) {
	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "model", "draft_mtp.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)

	// CtxOther is genuinely shared-KV-only: it is what makes the assistant
	// context borrow the target's llama_memory, and it is assigned from the
	// target *context* handle, not from the target's ContextParams. An own-KV
	// draft context must NOT set it. Everything else the shared loader
	// inherits describes cache geometry that both loaders need.
	exempt := map[string]bool{
		"CtxOther": true,
	}

	own := copiedCtxParamFields(t, fset, file, path, "loadDraftModelMTP")
	shared := copiedCtxParamFields(t, fset, file, path, "loadDraftModelMTPShared")

	var missing []string
	for field := range shared {
		if !exempt[field] && own[field] == token.NoPos {
			missing = append(missing, field)
		}
	}
	slices.Sort(missing)

	if len(missing) > 0 {
		ownFn := findKronkFunc(t, file, path, "loadDraftModelMTP")

		var detail strings.Builder
		for _, field := range missing {
			fmt.Fprintf(&detail, "\n              %-20s copied by loadDraftModelMTPShared at %s",
				field, srcPos(fset, root, shared[field]))
		}

		t.Errorf(`loadDraftModelMTP (%s) drops %d target context param(s) that loadDraftModelMTPShared copies: %s

  bug:      findings2 §6f — the own-KV embedded-MTP draft context does not
            inherit the target's KV cache topology.
  missing:%s
  copied:   %s
  why it matters:
            modelCtxParams sets KVUnified = 1 when nSeqMax > 1
            (sdk/kronk/model/config.go:936-939) and NCtx = ContextWindow*nSeqMax.
            At nSeqMax == 2 the target is unified with n_ctx_seq == 2*window,
            while the MTP draft inherits NCtx but stays non-unified with
            n_stream == 2, i.e. n_ctx_seq == window. Different topology and
            half the per-seq draft capacity.
            loadDraftModelMTPShared spells out the hazard at
            sdk/kronk/model/draft_mtp.go:330-337.
  exempt:   CtxOther (shared-KV only: it aliases the target's llama_memory and
            comes from the target context handle, not from ContextParams).`,
			srcPos(fset, root, ownFn.Pos()), len(missing), strings.Join(missing, ", "),
			detail.String(), strings.Join(slices.Sorted(maps.Keys(own)), ", "))
	}
}

// copiedCtxParamFields returns, for the function named name, every field
// assigned directly from the function's llama.ContextParams parameter, mapped
// to the position of the assignment.
func copiedCtxParamFields(t *testing.T, fset *token.FileSet, file *ast.File, path string, name string) map[string]token.Pos {
	t.Helper()

	fd := findKronkFunc(t, file, path, name)

	// Recover the identifier of the llama.ContextParams parameter rather than
	// hard-coding "targetCtxParams", so a rename does not silently pass.
	var src string
	for _, p := range fd.Type.Params.List {
		sel, ok := p.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		x, ok := sel.X.(*ast.Ident)
		if !ok || x.Name != "llama" || sel.Sel.Name != "ContextParams" {
			continue
		}
		if len(p.Names) > 0 {
			src = p.Names[0].Name
			break
		}
	}

	if src == "" {
		t.Fatalf("%s: %s has no named llama.ContextParams parameter; the test pins a signature that changed",
			fset.Position(fd.Pos()), name)
	}

	out := make(map[string]token.Pos)

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}

		for i, lhs := range assign.Lhs {
			if i >= len(assign.Rhs) {
				break
			}

			lsel, ok := lhs.(*ast.SelectorExpr)
			if !ok {
				continue
			}

			rsel, ok := assign.Rhs[i].(*ast.SelectorExpr)
			if !ok {
				continue
			}

			rx, ok := rsel.X.(*ast.Ident)
			if !ok || rx.Name != src {
				continue
			}

			out[lsel.Sel.Name] = lsel.Pos()
		}

		return true
	})

	if len(out) == 0 {
		t.Fatalf("%s: found no `x.F = %s.F` assignments in %s", fset.Position(fd.Pos()), src, name)
	}

	return out
}
