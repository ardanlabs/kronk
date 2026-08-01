package model

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestDocCommentClaimsMatchCode pins findings2 §8: doc comments that make a
// mechanically checkable claim about the code they annotate, and are wrong.
//
// Only claims that can be checked against the AST or the file itself are
// asserted here. Purely narrative staleness (for example
// batchgen_speculative.go:694-695 "we'll likely fail the slot below") is left
// to human review — a fake assertion would be worse than none.
//
// Each subtest is a pin, not a style check: it fails while the comment and the
// code disagree, and passes once either side is corrected.
func TestDocCommentClaimsMatchCode(t *testing.T) {
	root := kronkRepoRoot(t)

	tests := []struct {
		name  string
		check func(t *testing.T, root string)
	}{
		{"testlib CfgMTPChatMultiSlot flash-attention claim", checkTestlibFlashAttentionClaim},
		{"tests/mtp package doc NRsSeq claim", checkMTPSuiteNRsSeqClaim},
		{"draft_mtp shared MemorySeqRm no-op claim", checkSharedMemorySeqRmNoOpClaim},
		{"batchgen_tokens EOG line reference", checkBatchgenTokensEOGLineReference},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t, root)
		})
	}
}

// checkTestlibFlashAttentionClaim pins sdk/kronk/tests/testlib/testlib.go:394-395:
//
//	"Hybrid target requires f16 KV and disabled flash-attention (see
//	 config.go), inherited from the single-slot config."
//
// No such rule exists in config.go (verified: no branch in modelCtxParams or
// adjustConfig keys FlashAttention off hybrid / MTP), and the config the
// comment annotates never sets PtrFlashAttention. Config.FlashAttention()
// forwards to DerefFlashAttention, which returns FlashAttentionEnabled for a
// nil pointer (sdk/kronk/model/config.go:441-443, 1255-1260), so
// CfgMTPChatMultiSlot in fact runs with flash-attention ENABLED — the exact
// opposite of what the comment tells the next reader.
func checkTestlibFlashAttentionClaim(t *testing.T, root string) {
	// Ground the claim in the real default rather than restating it.
	if got := DerefFlashAttention(nil); got != FlashAttentionEnabled {
		t.Fatalf("precondition changed: DerefFlashAttention(nil) = %v, want FlashAttentionEnabled (config.go:1255)", got)
	}

	path := filepath.Join(root, "sdk", "kronk", "tests", "testlib", "testlib.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)
	fd := findKronkFunc(t, file, path, "CfgMTPChatMultiSlot")

	if fd.Doc == nil {
		return
	}

	doc := strings.ToLower(fd.Doc.Text())
	if !strings.Contains(doc, "disabled flash-attention") {
		return
	}

	// The doc claims flash-attention is disabled. That is only true if the
	// config literal actually sets PtrFlashAttention to a disabled value.
	var setsFA bool
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if key, ok := kv.Key.(*ast.Ident); ok && key.Name == "PtrFlashAttention" {
			setsFA = true
		}
		return true
	})

	if !setsFA {
		t.Errorf(`%s: doc comment on CfgMTPChatMultiSlot claims "disabled flash-attention (see config.go)" but the config never sets PtrFlashAttention.

  bug:      findings2 §8 / §5 — the comment is wrong in both halves.
  actual:   Config.FlashAttention() -> DerefFlashAttention(nil) ->
            FlashAttentionEnabled (sdk/kronk/model/config.go:441-443,
            1255-1260), so CfgMTPChatMultiSlot runs with FA ENABLED.
  also:     there is no rule anywhere in sdk/kronk/model/config.go that
            disables flash-attention for hybrid or MTP targets; the
            "(see config.go)" pointer resolves to nothing.
  measured: findings2 §5 — hybrid MTP loads and generates correctly with FA
            enabled at nSeqMax 1 and 2. The only real constraint is the
            inverse: FA *disabled* with a quantized V cache fails to load
            (.extras/llama.cpp/src/llama-context.cpp:458-462, 3561-3570).
  fix:      delete the claim, or set PtrFlashAttention explicitly.`,
			srcPos(fset, root, commentClaimPos(fd.Doc, "flash-attention")))
	}
}

// checkMTPSuiteNRsSeqClaim pins sdk/kronk/tests/mtp/suite_test.go:10-11:
//
//	"Recurrent-state snapshot allocation (NRsSeq) is logged by the loader and
//	 visible in test output."
//
// The loader does print an NRsSeq field (sdk/kronk/model/model.go:637-640) but
// it prints ctxParams.NRsSeq — the value kronk REQUESTED — and nothing in
// sdk/kronk/model/ ever assigns it (findings2 §4). The comment therefore
// advertises a diagnostic that is a hard-coded 0 for every run.
func checkMTPSuiteNRsSeqClaim(t *testing.T, root string) {
	path := filepath.Join(root, "sdk", "kronk", "tests", "mtp", "suite_test.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)

	if file.Doc == nil || !strings.Contains(file.Doc.Text(), "NRsSeq") {
		return
	}

	sites := fieldWriteSites(t, root, filepath.Join(root, "sdk", "kronk", "model"), "NRsSeq")
	if len(sites) == 0 {
		t.Errorf(`%s: package doc claims "Recurrent-state snapshot allocation (NRsSeq) is logged by the loader and visible in test output", but NRsSeq is never assigned in sdk/kronk/model/.

  bug:      findings2 §8 / §4.
  actual:   sdk/kronk/model/model.go:640 logs ctxParams.NRsSeq, the REQUESTED
            value. modelCtxParams (sdk/kronk/model/config.go:841) never sets
            it, so the logged value is 0 on every run and the suite's stated
            observability does not exist.
  fix:      set ctxParams.NRsSeq >= the MTP draft width and log the read-back
            llama.NRsSeq(lctx) (yzma .extras/yzma/pkg/llama/context.go:550),
            or delete the claim.`,
			srcPos(fset, root, commentClaimPos(file.Doc, "NRsSeq")))
	}
}

// checkSharedMemorySeqRmNoOpClaim pins sdk/kronk/model/draft_mtp.go:352:
//
//	"We keep the handle only for the safe ops (MemorySeqRm on the assistant's
//	 own cells is a no-op for the shared layers)."
//
// It is not a no-op. The comment sits inside loadDraftModelMTPShared, which
// builds the assistant context with params.CtxOther = targetCtx. The very next
// statement takes mem from llama.GetMemory(lctx), and for a CtxOther context
// that handle IS the target's llama_memory — the comment's own preceding
// sentence says so ("Shared memory: this returns the TARGET's llama_memory").
// So every MemorySeqRm through draft.core().mem for a *sharedMTPDrafter
// operates on the TARGET's KV. That is what makes findings2 §6g
// (finishSlot trimming the target memory, trimPos == 0 wiping the whole target
// sequence) a real latent trap rather than a theoretical one.
func checkSharedMemorySeqRmNoOpClaim(t *testing.T, root string) {
	path := filepath.Join(root, "sdk", "kronk", "model", "draft_mtp.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)
	fd := findKronkFunc(t, file, path, "loadDraftModelMTPShared")

	// Does this function alias the target's memory via CtxOther?
	var sharesMemory bool
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "CtxOther" {
				sharesMemory = true
			}
		}
		return true
	})

	if !sharesMemory {
		return
	}

	for _, group := range file.Comments {
		if group.Pos() < fd.Body.Pos() || group.End() > fd.Body.End() {
			continue
		}

		text := group.Text()
		if !strings.Contains(text, "MemorySeqRm") || !strings.Contains(text, "no-op") {
			continue
		}

		t.Errorf(`%s: comment claims "MemorySeqRm on the assistant's own cells is a no-op for the shared layers", but loadDraftModelMTPShared sets params.CtxOther = targetCtx.

  bug:      findings2 §8 / §6g.
  actual:   with CtxOther set, llama.GetMemory(lctx) returns the TARGET's
            llama_memory — the same comment block says so two sentences
            earlier ("this returns the TARGET's llama_memory"). Every
            MemorySeqRm via draft.core().mem for a *sharedMTPDrafter therefore
            mutates the target's KV, not assistant-private cells.
  impact:   finishSlot trims draft.core().mem; a trimPos of 0 wipes the entire
            target sequence. Currently masked only because the target seq is
            cleared right after anyway.
  fix:      state that MemorySeqRm through this handle hits the target's KV
            and must never be called with a shared-KV drafter.`,
			srcPos(fset, root, commentClaimPos(group, "no-op")))
	}
}

var eogLineRefRE = regexp.MustCompile(`line (\d+)`)

// checkBatchgenTokensEOGLineReference pins sdk/kronk/model/batchgen_tokens.go:183-184:
//
//	"Count every token the model generates. EOG tokens are handled before the
//	 state machine (line 55), so everything here is a real output token."
//
// The EOG handling site is llama.VocabIsEOG at line 66, not 55. Hard-coded line
// numbers rot on the first insertion above them; this asserts the pointer still
// resolves.
func checkBatchgenTokensEOGLineReference(t *testing.T, root string) {
	rel := filepath.Join("sdk", "kronk", "model", "batchgen_tokens.go")
	path := filepath.Join(root, rel)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	lines := strings.Split(string(raw), "\n")

	actual := 0
	for i, line := range lines {
		if strings.Contains(line, "llama.VocabIsEOG(") {
			actual = i + 1
			break
		}
	}
	if actual == 0 {
		t.Fatalf("%s: no llama.VocabIsEOG call found; the test pins a site that no longer exists", rel)
	}

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)

	for _, group := range file.Comments {
		text := group.Text()
		if !strings.Contains(text, "EOG tokens are handled") {
			continue
		}

		m := eogLineRefRE.FindStringSubmatch(text)
		if m == nil {
			continue
		}

		claimed, err := strconv.Atoi(m[1])
		if err != nil {
			t.Fatalf("%s: unparsable line reference %q", rel, m[1])
		}

		if !strings.Contains(lines[min(claimed, len(lines))-1], "llama.VocabIsEOG(") {
			t.Errorf(`%s: comment points at "line %d" for the EOG-handling site, but that site is line %d.

  bug:      findings2 §8 — stale hard-coded line reference.
  claimed:  %s:%d -> %q
  actual:   %s:%d -> %q
  fix:      reference handleSampledToken's VocabIsEOG branch by name, not by
            line number.`,
				srcPos(fset, root, commentClaimPos(group, m[0])), claimed, actual,
				rel, claimed, strings.TrimSpace(lines[min(claimed, len(lines))-1]),
				rel, actual, strings.TrimSpace(lines[actual-1]))
		}
	}
}
