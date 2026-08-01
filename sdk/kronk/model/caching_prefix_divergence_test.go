package model

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Finding 1: the IMC prompt-cache decode paths treat every llama_decode
// return code as success.
//
// FINDING
//
//	Both functions that decode a cached conversation prefix into a slot's KV
//	sequence throw away llama_decode's status code:
//
//	  sdk/kronk/model/caching_imc.go:118
//	      if _, err := llama.Decode(m.lctx, batch); err != nil {
//	  sdk/kronk/model/batchgen_mtp.go:407   (decodeTokensIntoCacheMTP, the
//	      MTP-aware analogue used for Qwen3.6 — see the dispatch at
//	      sdk/kronk/model/batchgen_slot_start.go:447-452)
//	      if _, err := llama.Decode(e.model.lctx, batch); err != nil {
//
//	yzma's binding returns the llama_decode status as the FIRST result and
//	only ever produces a non-nil error when the context handle is zero
//	(.extras/yzma/pkg/llama/context.go:382-390):
//
//	      func Decode(ctx Context, batch Batch) (int32, error) {
//	          if ctx == 0 { return 0, errInvalidContext }
//	          decodeFunc.Call(...)
//	          return int32(result), nil
//	      }
//
//	So for any live context the `err != nil` guard can never fire, and both
//	functions unconditionally return nil.
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/include/llama.h:964-976 documents the contract:
//	    0 - success
//	    1 - could not find a KV slot for the batch
//	    2 - aborted (processed ubatches remain in the memory state)
//	   -1 - invalid input batch
//	  < -1 - fatal error (processed ubatches remain in the memory state)
//
//	.extras/llama.cpp/tools/server/server-context.cpp:3639-3691 is the
//	reference handling: `const int ret = llama_decode(ctx_tgt, batch_view);`
//	followed by an explicit `if (ret != 0)` that classifies 1 / -1 / < -1,
//	calls `slot.prompt_clear()` (i.e. throws the cached prefix away) and
//	either errors the slot or retries with a smaller batch. Nothing in
//	llama.cpp ever advances a slot's n_past over a batch whose decode
//	returned non-zero.
//
// CONCRETE FAILURE SCENARIO
//
//	Multi-turn conversation, IMC "append" hit. startSlot restores the cached
//	prefix [0, reusable), then decodes the extension tokens through one of the
//	two functions above. The unified KV cache is under pressure from the other
//	slots, so find_slot fails and llama_decode returns 1 — the extension cells
//	were never written. Both functions report success, so startSlot continues:
//
//	  batchgen_slot_start.go:471  cacheIdx = llama.Pos(job.imcNewTotalCached)
//	  batchgen_slot_start.go:474  imcCommitSession(..., job.imcNewTotalCached,
//	                              ..., job.imcNewCachedTokens, ...)
//	  batchgen_slot_start.go:603  StateSeqGetData -> snapshot of a KV that is
//	                              missing the extension cells
//
//	The session now advertises N cached tokens backed by a KV holding fewer,
//	the suffix is decoded at position N (a hole in the sequence), and every
//	later turn of the conversation restores that same truncated snapshot while
//	believing the full prefix is present. The model attends over a prefix from
//	which whole messages are missing — it "contradicts what it already said"
//	because its own earlier context really did change under it. Identically for
//	ret == -1 on this hybrid IMROPE target (qwen35moe), where
//	llama_batch_allocr::init rejects a batch whose positions collide with the
//	sequence's pos_max (.extras/llama.cpp/src/llama-batch.cpp:255-270).
//
// This test fails while either call site discards the status code and passes
// once both bind and check it, matching the pattern already used by the sibling
// decoders in caching_imc_media.go.
func TestIMCCacheDecodeChecksLlamaDecodeStatus(t *testing.T) {
	root := kronkRepoRoot(t)
	modelDir := filepath.Join(root, "sdk", "kronk", "model")

	// Positive control: the sibling embedding decoder in the same package
	// binds and checks the status. If this ever stops being true the test
	// below is asserting a pattern that no longer exists in the codebase,
	// so fail loudly rather than silently passing.
	{
		fset := token.NewFileSet()
		path := filepath.Join(modelDir, "caching_imc_media.go")
		file := parseKronkSource(t, fset, path)
		fd := findKronkFunc(t, file, path, "decodeEmbeddingsIntoCache")

		site, ok := findLlamaDecodeSite(fd)
		if !ok {
			t.Fatalf("precondition changed: no llama.Decode call in decodeEmbeddingsIntoCache (%s)", path)
		}
		if site.discarded {
			t.Fatalf("precondition changed: decodeEmbeddingsIntoCache also discards the llama_decode status (%s)",
				srcPos(fset, root, site.pos))
		}
		if !llamaDecodeStatusIsCompared(fd, site.statusName) {
			t.Fatalf("precondition changed: decodeEmbeddingsIntoCache never compares %q (%s)",
				site.statusName, srcPos(fset, root, site.pos))
		}
	}

	tests := []struct {
		file string
		fn   string
		note string
	}{
		{
			file: "caching_imc.go",
			fn:   "decodeTokensIntoCache",
			note: "IMC text cache build/extend decode (called from batchgen_slot_start.go:451 and caching_imc_media.go:118)",
		},
		{
			file: "batchgen_mtp.go",
			fn:   "decodeTokensIntoCacheMTP",
			note: "IMC cache build/extend decode used whenever an MTP drafter is loaded (batchgen_slot_start.go:449) — the live path for Qwen3.6-35B-A3B-MTP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.fn, func(t *testing.T) {
			fset := token.NewFileSet()
			path := filepath.Join(modelDir, tt.file)
			file := parseKronkSource(t, fset, path)
			fd := findKronkFunc(t, file, path, tt.fn)

			site, ok := findLlamaDecodeSite(fd)
			if !ok {
				t.Fatalf("no llama.Decode call found in %s: the test pins a call site that no longer exists; re-point it", tt.fn)
			}

			switch {
			case site.discarded:
				t.Errorf(`%s: %s discards llama_decode's status code.

  bug:      the IMC prompt-cache decode reports success for every non-zero
            llama_decode return. %s
  actual:   the call is written as "_, err := llama.Decode(...)" and only err
            is checked, but yzma's Decode returns a non-nil error solely for a
            zero context handle
            (.extras/yzma/pkg/llama/context.go:382-390), so the guard is dead
            code for a live context and the function always returns nil.
  contract: .extras/llama.cpp/include/llama.h:964-976 — 1 = "could not find a
            KV slot for the batch", -1 = "invalid input batch",
            < -1 = fatal error.
  llama.cpp: tools/server/server-context.cpp:3639-3691 binds the status
            ("const int ret = llama_decode(...)"), classifies it, and calls
            slot.prompt_clear() instead of advancing n_past.
  impact:   startSlot then sets cacheIdx = job.imcNewTotalCached
            (batchgen_slot_start.go:471), commits that count to the IMC
            session (batchgen_slot_start.go:474) and snapshots a KV that is
            missing the just-"decoded" cells. Every later turn restores a
            prefix shorter than the session claims, so the conversation
            silently loses earlier context.
  fix:      bind the status ("ret, err := llama.Decode(...)") and fail on
            "err != nil || ret != 0" via decodeError(ret, err), exactly as
            decodeEmbeddingsIntoCache (caching_imc_media.go:236-244),
            decodeEmbeddingsMRoPEIntoCache (:312-323) and
            decodeTextMRoPEIntoCache (:377-388) already do.`,
					srcPos(fset, root, site.pos), tt.fn, tt.note)

			case !llamaDecodeStatusIsCompared(fd, site.statusName):
				t.Errorf(`%s: %s binds llama_decode's status as %q but never compares it.

  bug:      the status is captured and then ignored, which is equivalent to
            discarding it. %s
  fix:      fail the decode on "%s != 0" (see decodeEmbeddingsIntoCache,
            caching_imc_media.go:242-244).`,
					srcPos(fset, root, site.pos), tt.fn, site.statusName, tt.note, site.statusName)
			}
		})
	}
}

// llamaDecodeSite describes one `llama.Decode` call inside a function body.
type llamaDecodeSite struct {
	pos        token.Pos
	statusName string // identifier bound to the first (status) result
	discarded  bool   // true when the status result is assigned to "_"
}

// findLlamaDecodeSite returns the first `llama.Decode(...)` call in fd whose
// results are assigned, together with how the status result was bound.
func findLlamaDecodeSite(fd *ast.FuncDecl) (llamaDecodeSite, bool) {
	var out llamaDecodeSite
	var found bool

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		if found {
			return false
		}

		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
			return true
		}

		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}

		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Decode" {
			return true
		}
		if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "llama" {
			return true
		}

		out.pos = call.Pos()
		found = true

		lhs, ok := assign.Lhs[0].(*ast.Ident)
		if !ok {
			out.discarded = true
			return false
		}

		out.statusName = lhs.Name
		out.discarded = lhs.Name == "_"

		return false
	})

	return out, found
}

// llamaDecodeStatusIsCompared reports whether name appears on either side of an
// equality/inequality comparison anywhere in fd.
func llamaDecodeStatusIsCompared(fd *ast.FuncDecl, name string) bool {
	if name == "" || name == "_" {
		return false
	}

	var compared bool
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		if bin.Op != token.EQL && bin.Op != token.NEQ {
			return true
		}
		for _, side := range []ast.Expr{bin.X, bin.Y} {
			if id, ok := side.(*ast.Ident); ok && id.Name == name {
				compared = true
			}
		}
		return true
	})

	return compared
}

// =============================================================================
// Finding 2: the documented cachedRenderInputHash guard on the IMC pure-hit
// snapshot-skip path does not exist.
//
// FINDING
//
//	imcSession.cachedRenderInputHash is written at three sites
//	(sdk/kronk/model/caching_imc.go:159, :222, :262 — fed by
//	sdk/kronk/model/batchgen_slot_start.go:340, :474, :722-724) and is never
//	read in a comparison anywhere in sdk/kronk/model/. Three doc comments
//	nevertheless describe it as an active guard:
//
//	  sdk/kronk/model/model.go:94-96
//	      "cachedRenderInputHash guards token-v2 pure-hit snapshot reuse
//	       against changes to template inputs that are not represented by
//	       message tokens."
//	  sdk/kronk/model/caching.go:203-205
//	      "Used as the safety guard for IMCPureHitSnapshotSkip."
//	  sdk/kronk/model/batchgen_slot_start.go:153-160
//	      "A concurrent extend/rebuild between token-v2 planner and startSlot
//	       would change cachedMsgsHash / cachedMsgCount / totalTokensCached /
//	       cachedRenderInputHash"
//
//	The two staleness predicates the comments point at — sessionVersionOK
//	(batchgen_slot_start.go:169-176) and the skipSnapshot versionOK
//	(batchgen_slot_start.go:541-551) — compare cachedMsgsHash,
//	cachedMsgCount, totalTokensCached, logicalPosition, promptPlan, reserved
//	and kvState length. cachedRenderInputHash is absent from both. There is no
//	IMCPureHitSnapshotSkip symbol in the package either.
//
//	The existing "regression guard"
//	TestIMCCommitEmptyRenderHashDisqualifiesSkip
//	(sdk/kronk/model/caching_imc_pure_hit_test.go:308-323) states in its own
//	doc that "The startSlot block uses session.cachedRenderInputHash != "" as
//	one of its required guards" and then asserts only that imcCommitSession
//	stored the empty string — so the guard it claims to protect is never
//	exercised.
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/tools/server/server-task.cpp:1662-1690 and
//	tools/server/server-context.cpp:3197-3200 show the reference model for
//	prompt-cache validity: every field that can change what a cached prefix
//	means is folded into the comparison itself (server_tokens::get_common_prefix,
//	.extras/llama.cpp/tools/server/server-common.cpp:471-518, plus the explicit
//	alora_invocation_start clamp at server-context.cpp:3203-3207). llama.cpp
//	does not carry a guard field it never evaluates.
//
// CONCRETE FAILURE SCENARIO
//
//	Not a wrong-output bug today — the token-exact prefix comparison in
//	processIMCTokenPlan (caching_imc_tokens.go:56) and the promptPlan equality
//	check for media sessions (caching_imc_media_tokens.go:56-62) already
//	subsume every input the fingerprint covers. The defect is that three doc
//	comments and one test assert a safety guard that is not in the code, so the
//	next change to the pure-hit skip predicate (for example one that relaxes
//	the token comparison, or adds a template input that is not rendered into
//	tokens) will be made in the belief that the fingerprint still backstops it.
//
// This test fails while cachedRenderInputHash has zero comparison sites and
// passes once the guard is implemented (or the three claims and the vacuous
// regression guard are deleted).
func TestIMCPureHitSkipGuardReadsCachedRenderInputHash(t *testing.T) {
	root := kronkRepoRoot(t)
	modelDir := filepath.Join(root, "sdk", "kronk", "model")

	const field = "cachedRenderInputHash"

	// Ground the claim: only assert the missing guard while the doc comments
	// that advertise it are still present.
	claims := []struct {
		file string
		want string
	}{
		{"model.go", "guards token-v2 pure-hit snapshot reuse"},
		{"caching.go", "IMCPureHitSnapshotSkip"},
		{"batchgen_slot_start.go", "cachedRenderInputHash"},
	}

	var claimed []string
	for _, c := range claims {
		fset := token.NewFileSet()
		path := filepath.Join(modelDir, c.file)
		file := parseKronkSource(t, fset, path)

		for _, group := range file.Comments {
			if strings.Contains(group.Text(), c.want) {
				claimed = append(claimed, srcPos(fset, root, commentClaimPos(group, c.want)))
				break
			}
		}
	}

	if len(claimed) == 0 {
		t.Skipf("no doc comment advertises %s as a guard any more; nothing to pin", field)
	}

	writes := fieldWriteSites(t, root, modelDir, field)
	reads := fieldCompareSites(t, root, modelDir, field)

	if len(reads) == 0 {
		t.Errorf(`%s is written at %d site(s) and compared at 0 site(s), but %d doc comment(s) describe it as an active guard.

  bug:      the pure-hit snapshot-skip guard the comments promise does not
            exist in the code.
  claims:   %s
  writes:   %s
  actual:   neither sessionVersionOK (sdk/kronk/model/batchgen_slot_start.go:169-176)
            nor the skipSnapshot versionOK expression
            (sdk/kronk/model/batchgen_slot_start.go:541-551) mentions %s, and
            there is no IMCPureHitSnapshotSkip symbol in the package.
  also:     TestIMCCommitEmptyRenderHashDisqualifiesSkip
            (sdk/kronk/model/caching_imc_pure_hit_test.go:308-323) documents a
            guard ("startSlot ... uses session.cachedRenderInputHash != \"\"")
            that it never asserts, so the "regression guard" is vacuous.
  llama.cpp: tools/server/server-common.cpp:471-518 plus
            tools/server/server-context.cpp:3197-3207 — every input that can
            invalidate a cached prefix is part of the comparison itself; no
            unread guard field is carried alongside it.
  fix:      add session.cachedRenderInputHash == job.imcExpectedRenderHash &&
            session.cachedRenderInputHash != "" to both staleness predicates,
            or delete the claims above and the vacuous regression guard.`,
			field, len(writes), len(claimed),
			strings.Join(claimed, ", "),
			strings.Join(writes, ", "),
			field)
	}
}

// fieldCompareSites returns every repo-relative "file:line" in dir's non-test
// sources where a struct field named field appears on either side of an
// equality/inequality comparison.
func fieldCompareSites(t *testing.T, root string, dir string, field string) []string {
	t.Helper()

	fset := token.NewFileSet()

	var sites []string
	for _, path := range nonTestGoFiles(t, dir) {
		f := parseKronkSource(t, fset, path)

		ast.Inspect(f, func(n ast.Node) bool {
			bin, ok := n.(*ast.BinaryExpr)
			if !ok {
				return true
			}
			if bin.Op != token.EQL && bin.Op != token.NEQ {
				return true
			}
			for _, side := range []ast.Expr{bin.X, bin.Y} {
				if sel, ok := side.(*ast.SelectorExpr); ok && sel.Sel.Name == field {
					sites = append(sites, srcPos(fset, root, sel.Pos()))
				}
			}
			return true
		})
	}

	return sites
}
