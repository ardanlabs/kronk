package model

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// batchseq (sequence-batch) engine regressions.
//
// SCOPE NOTE: the batchseq engine is reached ONLY from (*Model).Embeddings
// (sdk/kronk/model/embed.go:91) and (*Model).Rerank (sdk/kronk/model/rerank.go:95),
// selected at sdk/kronk/model/model.go:376 via useBatchSeq ->
// supportsBatchSeq (sdk/kronk/model/batchseq_compat.go:7), which is true only
// for mi.IsEmbedModel || mi.IsRerankModel. Chat / ChatStreaming never touch it.
// Both regressions pinned below therefore affect embedding and reranking
// quality, not text generation.
//
// Both live in code paths that need a loaded llama shared library plus a loaded
// GGUF (llama.Decode / llama.GetEmbeddingsSeq / llama.Tokenize are raw FFI
// calls), so they are pinned as AST assertions over the checked-in source in
// the style of mtp_ctxparams_source_test.go and doc_comment_claims_test.go.
// They locate everything by declaration name, never by line number, so they
// survive unrelated edits. Shared helpers (kronkRepoRoot, parseKronkSource,
// findKronkFunc, srcPos, nonTestGoFiles) come from
// sdk/kronk/model/mtp_ctxparams_source_test.go.

// batchSeqSelectorNames collects every `pkg.Name` selector name used anywhere in
// the non-test sources of dir. Only the trailing identifier is recorded, so
// `llama.GetEmbeddingsIth` is reported as "GetEmbeddingsIth".
func batchSeqSelectorNames(t *testing.T, dir string) map[string]string {
	t.Helper()

	root := kronkRepoRoot(t)
	fset := token.NewFileSet()
	found := make(map[string]string)

	for _, path := range nonTestGoFiles(t, dir) {
		f := parseKronkSource(t, fset, path)

		ast.Inspect(f, func(n ast.Node) bool {
			sel, ok := n.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			if _, ok := sel.X.(*ast.Ident); !ok {
				return true
			}
			if _, exists := found[sel.Sel.Name]; !exists {
				found[sel.Sel.Name] = srcPos(fset, root, sel.Pos())
			}

			return true
		})
	}

	return found
}

// batchSeqFuncSelectors collects the `pkg.Name` selector names used inside the
// single function declaration named fn in the file at path.
func batchSeqFuncSelectors(t *testing.T, path string, fn string) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	f := parseKronkSource(t, fset, path)
	decl := findKronkFunc(t, f, path, fn)

	found := make(map[string]bool)
	ast.Inspect(decl, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			found[sel.Sel.Name] = true
		}

		return true
	})

	return found
}

// =============================================================================

// TestBatchSeqEmbeddingHandlesNonePooling pins the missing
// LLAMA_POOLING_TYPE_NONE path in the sequence-batch embedding runtime.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchseq_engine.go:476  (*batchSeqEngine).evaluateJob
//     unconditionally reads its per-item output with
//     `llama.GetEmbeddingsSeq(e.lctx, entry.seqID, int32(job.outputWidth))`.
//   - sdk/kronk/model/batchseq_engine.go:478  classifies that read failing as
//     `fatal = true`.
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/tools/server/server-context.cpp:2202-2207
//     (send_embedding) branches on the resolved pooling type:
//     LLAMA_POOLING_TYPE_NONE -> llama_get_embeddings_ith(ctx, i);
//     anything else -> llama_get_embeddings_seq(ctx, seq_id).
//     :2209-2214 then degrades a NULL result to a zero vector for that ONE
//     request instead of tearing anything down.
//   - .extras/llama.cpp/tools/server/server-context.cpp:2242-2245
//     (send_rerank) keeps the same llama_get_embeddings_ith fallback.
//   - .extras/llama.cpp/src/llama-context.cpp:1489-1531 (encode) and
//     :1934-1981 (decode) populate ctx->embd_seq for MEAN / CLS / LAST / RANK
//     ONLY. Under LLAMA_POOLING_TYPE_NONE the per-token buffer embd.data is
//     filled instead and embd_seq stays empty, so
//     .extras/llama.cpp/src/llama-context.cpp:924-931 (get_embeddings_seq)
//     returns nullptr for every sequence id.
//   - .extras/llama.cpp/src/llama-context.cpp:232-236 resolves
//     LLAMA_POOLING_TYPE_UNSPECIFIED (the llama_context_default_params value,
//     :3491) to LLAMA_POOLING_TYPE_NONE whenever the model's own
//     hparams.pooling_type is also UNSPECIFIED. The hparams default is
//     LLAMA_POOLING_TYPE_NONE (.extras/llama.cpp/src/llama-hparams.h:272) and
//     the `<arch>.pooling_type` GGUF key is only written when the source HF
//     repo ships a sentence-transformers 1_Pooling/config.json
//     (.extras/llama.cpp/conversion/base.py:2062-2075).
//
// FAILURE SCENARIO: load any embedding GGUF whose metadata lacks
// `<arch>.pooling_type` — third-party requantizations routinely drop it, and
// kronk never supplies a default of its own (sdk/kronk/model/config.go:844-850
// sets ctxParams.PoolingType for rerank models only, leaving embedding models
// at UNSPECIFIED). llama_decode succeeds and returns 0, so the status check at
// batchseq_engine.go:467-473 is happy. Then GetEmbeddingsSeq returns a nil
// slice (yzma .extras/yzma/pkg/llama/context.go:451-462 maps the NULL pointer
// to `nil, nil`), the length guard at batchseq_engine.go:480 reports
// "item[N] returned 0 outputs, expected D", and because that error is raised
// with fatal=true the engine's processLoop terminates permanently
// (sdk/kronk/model/batchseq_engine.go:343-347 -> terminate). m.batchSeq is
// never rebuilt, so EVERY later Embeddings() call on that model fails with the
// stored engine error. Upstream serves the request from embd.data instead.
//
// THE ASSERTION: nothing in package model references the NONE-pooling half of
// the llama API — neither the llama.GetEmbeddingsIth fallback nor
// llama.GetPoolingType, nor any non-RANK llama.PoolingType* constant that would
// let kronk pin a pooling mode at context creation. Either fix satisfies this
// test.
func TestBatchSeqEmbeddingHandlesNonePooling(t *testing.T) {
	root := kronkRepoRoot(t)
	dir := filepath.Join(root, "sdk", "kronk", "model")

	// Sanity: the pooled-only accessor really is what evaluateJob uses. If this
	// fails the production code was restructured and the assertion below needs
	// to be re-derived rather than trusted.
	sels := batchSeqFuncSelectors(t, filepath.Join(dir, "batchseq_engine.go"), "evaluateJob")
	if !sels["GetEmbeddingsSeq"] {
		t.Fatal("evaluateJob no longer calls llama.GetEmbeddingsSeq; re-derive this assertion")
	}

	// Any one of these proves the NONE-pooling case is handled: the upstream
	// per-token fallback, a runtime pooling-type query, or forcing a pooled mode
	// on the context for embedding models.
	remedies := []string{
		"GetEmbeddingsIth",
		"GetPoolingType",
		"PoolingTypeMean",
		"PoolingTypeCLS",
		"PoolingTypeLast",
		"PoolingTypeNone",
	}

	names := batchSeqSelectorNames(t, dir)
	for _, remedy := range remedies {
		if at, ok := names[remedy]; ok {
			t.Logf("NONE-pooling handled via llama.%s at %s", remedy, at)
			return
		}
	}

	t.Fatalf("package model references none of %v: the sequence-batch embedding "+
		"path only reads llama.GetEmbeddingsSeq, which returns NULL under "+
		"LLAMA_POOLING_TYPE_NONE, and treats that as a fatal engine error "+
		"(batchseq_engine.go:476-482). Upstream send_embedding falls back to "+
		"llama_get_embeddings_ith (server-context.cpp:2202-2207).", remedies)
}

// TestFormatRerankPairSeparatesQueryFromDocument pins the malformed
// cross-encoder input built for every reranking request.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/rerank.go:261-263  formatRerankPair returns
//     `fmt.Sprintf("%s %s", query, document)` — a bare space join.
//   - sdk/kronk/model/rerank.go:157-158  (processRerankBatchSeq, the batchseq
//     feeder) tokenizes that single string with
//     `llama.Tokenize(m.vocab, pairText, m.addBOSToken, true)`.
//   - sdk/kronk/model/rerank.go:209-211  the context-pool fallback does the
//     same, so both runtimes are affected.
//
// LLAMA.CPP REFERENCE
//   - .extras/llama.cpp/tools/server/server-common.h:378 states the contract
//     verbatim: "format rerank task: [BOS]query[EOS][SEP]doc[EOS]".
//   - .extras/llama.cpp/tools/server/server-common.cpp:1553-1594
//     (format_prompt_rerank) implements it at the TOKEN level: it prefers the
//     model's own "rerank" chat template (llama_model_chat_template(model,
//     "rerank"), :1561-1568) and otherwise assembles
//     BOS + tokenize(query) + EOS + SEP + tokenize(doc) + EOS, gated on
//     llama_vocab_get_add_bos / _add_eos / _add_sep (:1570-1591).
//   - .extras/llama.cpp/tools/server/server-context.cpp:5022 is the only
//     producer of rerank prompts in the server.
//
// FAILURE SCENARIO: bge-reranker-v2-m3 (the reranker this repo tests with,
// sdk/kronk/tests/testlib/testlib.go:68) is an XLM-RoBERTa cross-encoder
// trained on `<s> query </s></s> document </s>` — the doubled separator is the
// segment boundary the model uses to tell the two halves apart. kronk hands it
// `<s> query document </s>` instead (llama.Tokenize's addSpecial only adds the
// vocab's BOS/EOS, never an interior SEP), so the model scores one run-on
// sentence. Nothing errors: LLAMA_POOLING_TYPE_RANK still emits an n_cls_out
// score, sigmoid() still maps it into [0,1], and Rerank() still returns a
// confident-looking ordering that is silently wrong. A model-side "rerank"
// chat template, if the GGUF carries one, is likewise ignored.
//
// THE ASSERTION, part 1: formatRerankPair is a pure function, so the wrong
// output is asserted directly. Part 2: no rerank code path consults the vocab
// separator or the "rerank" chat template, which is why the space join cannot
// be corrected inside formatRerankPair alone — the fix has to move the pair
// assembly to token level.
func TestFormatRerankPairSeparatesQueryFromDocument(t *testing.T) {
	const query = "what is the capital of France"
	const document = "Paris is the capital and most populous city of France."

	got := formatRerankPair(query, document)

	if got == query+" "+document {
		t.Errorf("formatRerankPair joined the pair with a bare space and no segment separator:\n"+
			"\tgot: %q\n"+
			"\tllama.cpp emits [BOS]query[EOS][SEP]doc[EOS] "+
			"(.extras/llama.cpp/tools/server/server-common.cpp:1570-1591)", got)
	}

	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "model", "rerank.go")

	fset := token.NewFileSet()
	f := parseKronkSource(t, fset, path)

	// Either the vocab separator or the model's "rerank" chat template has to
	// show up somewhere in rerank.go for the pair to be assembled correctly.
	// llama.VocabSEP / llama.VocabGetAddSEP / llama.ModelChatTemplate are all
	// already bound by yzma (.extras/yzma/pkg/llama/vocab.go:281, :341 and
	// .extras/yzma/pkg/llama/model.go), so none of these is a missing binding.
	remedies := []string{"VocabSEP", "VocabGetAddSEP", "ModelChatTemplate"}

	seen := make(map[string]bool, len(remedies))
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			seen[sel.Sel.Name] = true
		}

		return true
	})

	for _, remedy := range remedies {
		if seen[remedy] {
			t.Logf("rerank pair assembly consults llama.%s", remedy)

			return
		}
	}

	t.Errorf("rerank.go references none of %s: the query/document pair is built "+
		"as a plain string join (rerank.go:261-263) and tokenized in one shot "+
		"(rerank.go:157-158, rerank.go:209-211), so the [EOS][SEP] segment "+
		"boundary required by llama.cpp's format_prompt_rerank "+
		"(server-common.cpp:1553-1594, contract at server-common.h:378) is "+
		"never emitted", strings.Join(remedies, ", "))
}
