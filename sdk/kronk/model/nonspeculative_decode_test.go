package model

import (
	"go/ast"
	"go/token"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// =============================================================================
// PLAIN (NON-SPECULATIVE) DECODE PATH REGRESSIONS
//
// Every finding pinned in this file is reachable with NO drafter configured
// (e.model.draft == nil), i.e. it survives "disable MTP" as a workaround.
//
// A plain Chat() always lands in the generation batch engine: batchseq_engine.go
// is a separate runtime used only by embed.go:33 and rerank.go:35, and it never
// samples. So batchgen_*.go is the only generation path.
//
// Most of these live inside functions that need a live llama.Context and a
// loaded GGUF (llama.SamplerSample, llama.Decode, llama.GetLogitsIth), so where
// a direct call is impossible the test is either a VERBATIM MIRROR of the
// production arithmetic/ordering (with the mirrored file.go:LINE cited on the
// mirror itself, per rollback_draft_test.go's convention) or an AST assertion
// over the checked-in source.
// =============================================================================

// -----------------------------------------------------------------------------
// Finding 1: every sampled token is accepted TWICE into the sampler chain.

// penaltyRingMirror is a VERBATIM MIRROR of llama.cpp's
// `ring_buffer<llama_token> prev` as the penalties sampler uses it:
//
//	.extras/llama.cpp/src/llama-sampler.cpp:25-26   ring_buffer(size_t cap) : capacity(cap), data(cap) {}
//	.extras/llama.cpp/src/llama-sampler.cpp:55-68   push_back: when sz == capacity, advance `first` (evict oldest)
//	.extras/llama.cpp/src/llama-sampler.cpp:2782    prev = ring_buffer<llama_token>(penalty_last_n)
//
// The capacity is penalty_last_n, so the buffer holds at most penalty_last_n
// ACCEPTED tokens — not at most penalty_last_n *generated* tokens.
type penaltyRingMirror struct {
	capacity int
	items    []int
}

func (r *penaltyRingMirror) size() int { return len(r.items) }

func (r *penaltyRingMirror) front() int { return r.items[0] }

func (r *penaltyRingMirror) pushBack(tok int) {
	if r.capacity == 0 {
		return
	}
	if len(r.items) == r.capacity {
		r.items = r.items[1:]
	}
	r.items = append(r.items, tok)
}

// penaltiesMirror is a VERBATIM MIRROR of llama_sampler_penalties' state and of
// llama_sampler_penalties_accept:
//
//	.extras/llama.cpp/src/llama-sampler.cpp:2657-2685
//	    ctx->token_count[token]++;
//	    if (ctx->prev.size() >= (size_t) ctx->penalty_last_n) {
//	        const auto old = ctx->prev.front();
//	        ctx->token_count[old]--;
//	        if (ctx->token_count[old] == 0) { ctx->token_count.erase(old); }
//	    }
//	    ctx->prev.push_back(token);
//
// llama_sampler_penalties_apply (:2687-2718) reads token_count: the repeat
// penalty is applied once per PRESENT token (membership only), the presence
// penalty is `float(count > 0) * penalty_present` (membership only), but the
// frequency penalty is `float(count) * penalty_freq` — proportional to the
// stored count.
type penaltiesMirror struct {
	penaltyLastN int
	prev         penaltyRingMirror
	tokenCount   map[int]int
}

func newPenaltiesMirror(penaltyLastN int) *penaltiesMirror {
	return &penaltiesMirror{
		penaltyLastN: penaltyLastN,
		prev:         penaltyRingMirror{capacity: penaltyLastN},
		tokenCount:   make(map[int]int),
	}
}

func (p *penaltiesMirror) accept(tok int) {
	if p.penaltyLastN == 0 {
		return
	}

	p.tokenCount[tok]++

	if p.prev.size() >= p.penaltyLastN {
		old := p.prev.front()
		p.tokenCount[old]--
		if p.tokenCount[old] == 0 {
			delete(p.tokenCount, old)
		}
	}

	p.prev.pushBack(tok)
}

// distinctWindow reports how many DISTINCT emitted tokens the penalty window
// still covers, which is the property `penalty_last_n = 64` is supposed to
// guarantee.
func (p *penaltiesMirror) distinctWindow() int {
	return len(p.tokenCount)
}

// TestSamplerAcceptedTwicePerTokenCorruptsPenaltyWindow pins the double-accept
// on the PLAIN, non-speculative decode path.
//
// FINDING
// llama_sampler_sample() itself ends with llama_sampler_accept(smpl, token)
// (.extras/llama.cpp/src/llama-sampler.cpp:873, inside the function that starts
// at :810). Kronk calls it through yzma's thin wrapper llama.SamplerSample
// (.extras/yzma/pkg/llama/sampling.go:574-584) and then calls
// llama.SamplerAccept on the SAME chain again, so the chain's accept() runs
// twice for every emitted token.
//
// PRODUCTION LINES PINNED (all non-speculative)
//   - sdk/kronk/model/batchgen_tokens.go:25  processSlotToken   -> :63 accept
//   - sdk/kronk/model/batchgen_tokens.go:265 sampleFirstToken   -> :63 accept
//   - sdk/kronk/model/batchgen_engine.go:259 M-RoPE generation  -> :261 -> :63
//
// The single accept is at sdk/kronk/model/batchgen_tokens.go:63
// (`llama.SamplerAccept(s.sampler, token)`), which is the ONLY SamplerAccept
// call on the request sampler in the whole package.
//
// LLAMA.CPP REFERENCE (correct behaviour)
//   - .extras/llama.cpp/common/sampling.cpp:646-676
//     common_sampler_sample_and_accept_n uses common_sampler_sample (which
//     calls llama_sampler_apply, NOT llama_sampler_sample — see
//     common/sampling.cpp:630-643) plus exactly ONE common_sampler_accept per
//     emitted token.
//   - .extras/llama.cpp/tools/server/server-context.cpp:3801 + :3806
//     the plain server path: common_sampler_sample() then exactly one
//     common_sampler_accept().
//
// FAILURE SCENARIO
// With Params.RepeatLastN = 64 (DefRepeatLastN, sdk/kronk/model/params.go:80)
// and any repeat/frequency/presence penalty set, the penalties sampler is added
// to the chain (sdk/kronk/model/params.go:785-788). Its ring buffer capacity is
// penalty_last_n, so two accepts per token mean the window covers only 32 real
// output tokens instead of 64, and token_count reaches 2 for every token in the
// window, doubling the frequency-penalty weight `float(count) * penalty_freq`
// (.extras/llama.cpp/src/llama-sampler.cpp:2715). The repeat and presence
// penalties are membership-only and keep their nominal weight.
//
// The same double-accept halves the DRY sampler's last_tokens window
// (.extras/llama.cpp/src/llama-sampler.cpp:3245, accept at :2937-2944) and
// additionally poisons its content: DRY's suffix-repetition search runs over
// "a a b b c c" instead of "a b c", so it detects repetitions the model never
// produced.
//
// The default chain does NOT contain penalties or DRY (DefRepeatPenalty = 1.0,
// DefFrequencyPenalty = 0, DefPresencePenalty = 0, DefDryMultiplier = 0 at
// sdk/kronk/model/params.go:34/47/72/88, gated at params.go:785 and :792), and
// the remaining default samplers (suppress-bias, top-k, top-p, min-p, temp-ext,
// dist) all have `.accept = nullptr`. So this is latent under default params
// and bites any caller that enables a penalty.
//
// FIX: use llama.SamplerApply on a locally built candidate array (as
// common_sampler_sample does) and keep the single explicit
// llama.SamplerAccept, or drop the explicit accept at batchgen_tokens.go:63.
func TestSamplerAcceptedTwicePerTokenCorruptsPenaltyWindow(t *testing.T) {
	const (
		penaltyLastN = 64 // DefRepeatLastN, sdk/kronk/model/params.go:80
		nEmitted     = 64
	)

	// One distinct token per emitted position, so the window content is
	// unambiguous.
	emitted := make([]int, nEmitted)
	for i := range emitted {
		emitted[i] = 1000 + i
	}

	// Upstream: common_sampler_sample (apply only) + one accept per token.
	upstream := newPenaltiesMirror(penaltyLastN)
	for _, tok := range emitted {
		upstream.accept(tok)
	}

	// Kronk: llama.SamplerSample accepts internally
	// (llama-sampler.cpp:873), then batchgen_tokens.go:63 accepts again.
	kronk := newPenaltiesMirror(penaltyLastN)
	for _, tok := range emitted {
		kronk.accept(tok) // implicit, inside llama_sampler_sample
		kronk.accept(tok) // explicit, batchgen_tokens.go:63
	}

	if upstream.distinctWindow() != nEmitted {
		t.Fatalf("mirror is wrong: upstream window covers %d of %d emitted tokens",
			upstream.distinctWindow(), nEmitted)
	}

	if got := kronk.distinctWindow(); got != upstream.distinctWindow() {
		t.Errorf("penalty window after %d emitted tokens with penalty_last_n=%d: kronk covers %d distinct tokens, upstream covers %d\n"+
			"llama.SamplerSample already runs llama_sampler_accept (llama-sampler.cpp:873); "+
			"batchgen_tokens.go:63 accepts the same token a second time, so the "+
			"ring_buffer of capacity penalty_last_n (llama-sampler.cpp:2782) holds two "+
			"entries per output token and the effective repetition window is halved.",
			nEmitted, penaltyLastN, got, upstream.distinctWindow())
	}

	// Weight: token_count is what the frequency penalty is multiplied by
	// (llama-sampler.cpp:2715).
	{
		tok := emitted[nEmitted-1]
		want := upstream.tokenCount[tok]
		if got := kronk.tokenCount[tok]; got != want {
			t.Errorf("token_count[%d] = %d, want %d\n"+
				"the doubled accept double-counts each token, so the frequency penalty "+
				"`float(count) * penalty_freq` (llama-sampler.cpp:2715) is applied at twice "+
				"the configured strength.", tok, got, want)
		}
	}
}

// TestPromptTokensArePrimedIntoTheSamplerChain pins the second half of the
// sampler-state divergence: Kronk never seeds the sampler chain with the prompt.
//
// FINDING
// llama.cpp's server primes the request's sampler with every prompt token
// before generation starts:
//
//	.extras/llama.cpp/tools/server/server-context.cpp:375-397  server_slot::init_sampler()
//	    common_sampler_reset(smpl.get());
//	    for (int i = 0; i < (int) prompt.tokens.size(); i++) {
//	        const llama_token id = prompt.tokens[i];
//	        if (id != LLAMA_TOKEN_NULL) { common_sampler_accept(smpl.get(), id, false); }
//	    }
//
// so penalties / DRY see the last penalty_last_n tokens of the
// prompt-plus-generation stream, exactly as the sampler's own documentation
// assumes.
//
// PRODUCTION LINE PINNED: sdk/kronk/model/batchgen_tokens.go:63 is the only
// llama.SamplerAccept call on a request sampler anywhere in the package (the
// other two are grammar.go:436, a different sampler object, and
// batchgen_speculative.go:198, a SamplerReset on the draft sampler). Neither
// startSlotText (sdk/kronk/model/batchgen_slot_start.go:791) nor
// addPrefillChunk (sdk/kronk/model/batchgen_prefill_text.go:14) primes the
// chain.
//
// FAILURE SCENARIO
// In a long multi-turn conversation the entire history is prompt, not
// generation. With RepeatPenalty/FrequencyPenalty/DRY enabled the penalty
// window starts EMPTY at the first output token, so nothing discourages the
// model from re-deriving or restating text that is already in the transcript —
// and on a fresh turn it has no memory at all of what it said in the previous
// turn. Combined with the doubled accept above, the window that does build up
// is half the configured length.
//
// FIX: after building s.sampler in startSlot, accept the full generation-ready
// token sequence (job.actualTokens, or the cached prefix plus job.tailTokens)
// into the chain, mirroring init_sampler().
func TestPromptTokensArePrimedIntoTheSamplerChain(t *testing.T) {
	fset, files := parseModelPackage(t)

	// enclosing function name -> "file:line" of each llama.SamplerAccept call
	// made on a slot's request sampler (s.sampler).
	sites := make(map[string][]string)

	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}

			ast.Inspect(fd.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isLlamaCall(call, "SamplerAccept") || len(call.Args) == 0 {
					return true
				}

				sel, ok := call.Args[0].(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "sampler" {
					return true
				}

				sites[fd.Name.Name] = append(sites[fd.Name.Name], posOf(fset, call))

				return true
			})
		}
	}

	if len(sites) == 0 {
		t.Fatal("no llama.SamplerAccept(x.sampler, ...) call found at all; " +
			"this assertion has drifted from the source")
	}

	// A prompt-priming site must exist somewhere on the slot-start / prefill
	// path, before the first token is generated.
	priming := []string{"startSlot", "startSlotText", "primeSampler", "addPrefillChunk", "toSampler"}

	for _, name := range priming {
		if _, ok := sites[name]; ok {
			return
		}
	}

	var found []string
	for name, at := range sites {
		found = append(found, name+" ("+strings.Join(at, ", ")+")")
	}

	t.Errorf("no prompt-token priming of the request sampler chain; llama.SamplerAccept(*.sampler, ...) only appears in: %s\n"+
		"llama.cpp primes the chain with every prompt token in server_slot::init_sampler "+
		"(.extras/llama.cpp/tools/server/server-context.cpp:375-397) so the penalties and "+
		"DRY windows cover prompt+generation. Kronk only accepts tokens it generated itself "+
		"(sdk/kronk/model/batchgen_tokens.go:63), so in a multi-turn conversation the entire "+
		"transcript is invisible to the repetition machinery.",
		strings.Join(found, "; "))
}

// -----------------------------------------------------------------------------
// Finding 2: a slot released mid-batch-assembly leaves its rows in the pending
// batch, and fillSlots can hand the same KV sequence to a new request before
// the decode runs.

// mirrorBatchRow is one row of the shared llama_batch: token, position, and the
// single sequence id batch.Add writes (sdk/kronk/model/batchgen_prefill_text.go:63,
// via slot.seqIDs, sdk/kronk/model/batchgen_engine.go:54). `req` is test-only
// bookkeeping identifying which request produced the row.
type mirrorBatchRow struct {
	pos int
	seq int
	req string
}

// mirrorGenSlot mirrors the subset of `slot` the batch-assembly ordering
// touches (sdk/kronk/model/batchgen_slot.go:84-298).
type mirrorGenSlot struct {
	seq        int
	active     bool
	req        string
	nPast      int
	prefill    int // len(s.prefillTokens)
	nPrefilled int
	cancelled  bool // s.job.ctx.Err() != nil
}

// mirrorAssembleBatch is a VERBATIM MIRROR of the batch-assembly ordering in
// (*batchEngine).processBatch, for a text-only, non-speculative, non-media
// configuration (e.model.draft == nil, no M-RoPE slots):
//
//	sdk/kronk/model/batchgen_engine.go:201       e.batch.Clear()
//	sdk/kronk/model/batchgen_engine.go:366-396   round-robin prefill loop
//	sdk/kronk/model/batchgen_engine.go:373-376     cancel check -> e.finishSlot(s, ...)
//	sdk/kronk/model/batchgen_prefill_text.go:60-66 add chunk rows, s.nPast++
//	sdk/kronk/model/batchgen_engine.go:418       e.fillSlots(buf)
//	sdk/kronk/model/batchgen_schedule.go:51-58     first inactive slot wins
//	sdk/kronk/model/batchgen_slot_start.go:493-498 clear the seq, s.nPast = cacheIdx
//	sdk/kronk/model/batchgen_slot_start.go:945-949 addPrefillChunk for the new job
//	sdk/kronk/model/batchgen_engine.go:458       llama.Decode(e.model.lctx, e.batch)
//
// finishSlot's KV teardown is sdk/kronk/model/batchgen_finish.go:176-182
// (MemorySeqRm(seqID, -1, -1)) followed by s.reset()
// (sdk/kronk/model/batchgen_slot.go:300-321, which clears nPast and active).
// Neither touches e.batch.
//
// MAINTAINER: when processBatch is fixed, update this mirror in the same commit.
func mirrorAssembleBatch(slots []*mirrorGenSlot, chunkLimit int, nBatch int, pending []string, cancelAfterPass int) []mirrorBatchRow {
	var batch []mirrorBatchRow

	// batchgen_prefill_text.go:14-86.
	addPrefillChunk := func(s *mirrorGenSlot) {
		if s.prefill == 0 || s.nPrefilled >= s.prefill {
			return
		}

		available := nBatch - len(batch)
		if available <= 0 {
			return
		}

		chunk := min(s.prefill-s.nPrefilled, available, chunkLimit)
		for range chunk {
			batch = append(batch, mirrorBatchRow{pos: s.nPast, seq: s.seq, req: s.req})
			s.nPast++
		}
		s.nPrefilled += chunk
	}

	// batchgen_finish.go:176-182 + batchgen_slot.go:300-321. The already-added
	// rows in `batch` are deliberately left untouched: that is the bug.
	finishSlot := func(s *mirrorGenSlot) {
		s.active = false
		s.nPast = 0
		s.prefill = 0
		s.nPrefilled = 0
		s.req = ""
	}

	// batchgen_engine.go:366-396.
	pass := 0
	for {
		before := len(batch)
		pass++

		for _, s := range slots {
			if !s.active || s.prefill == 0 {
				continue
			}

			// batchgen_engine.go:373-377.
			if s.cancelled && pass > cancelAfterPass {
				finishSlot(s)
				continue
			}

			addPrefillChunk(s)
		}

		if len(batch) == before {
			break
		}
	}

	// batchgen_engine.go:418 -> batchgen_schedule.go:43-63.
	for _, job := range pending {
		for _, s := range slots {
			if s.active {
				continue
			}

			// batchgen_slot_start.go:493-498: clear the seq, nPast = cacheIdx
			// (0 for a non-IMC request), then add the first chunk (:945-949).
			s.active = true
			s.req = job
			s.nPast = 0
			s.prefill = chunkLimit
			s.nPrefilled = 0
			s.cancelled = false
			addPrefillChunk(s)

			break
		}
	}

	return batch
}

// TestReleasedSlotRowsAreRemovedFromPendingBatch pins the cross-request KV
// contamination window in the plain decode path.
//
// FINDING
// (*batchEngine).processBatch assembles the shared batch, releases slots in the
// middle of that assembly, and then calls fillSlots — which can immediately
// re-assign the released slot (and therefore its KV sequence id) to a queued
// request — all BEFORE the single llama.Decode. The released request's rows are
// never removed from e.batch, so one llama_decode writes two different
// requests' tokens into the same KV sequence.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_engine.go:373-383  finishSlot inside the
//     round-robin prefill loop, after earlier passes already pushed rows for
//     that slot's seqID into e.batch (batchgen_prefill_text.go:63).
//   - sdk/kronk/model/batchgen_finish.go:176-182  clears the KV sequence but
//     leaves e.batch alone; s.reset() (batchgen_slot.go:300-321) drops nPast.
//   - sdk/kronk/model/batchgen_engine.go:418      fillSlots runs BEFORE the
//     decode and takes the first inactive slot (batchgen_schedule.go:51-58).
//   - sdk/kronk/model/batchgen_engine.go:458      the decode that commits both
//     requests' rows to the same sequence.
//
// LLAMA.CPP REFERENCE (correct behaviour)
//   - .extras/llama.cpp/tools/server/server-queue.cpp:139-163
//     start_loop() drains every queued task (which is where
//     launch_slot_with_task assigns slots) and only THEN calls
//     callback_update_slots(); the comment on :159 is literally "all tasks in
//     the current loop is processed, slots data is now ready". Slot assignment
//     can therefore never interleave with batch assembly.
//   - .extras/llama.cpp/tools/server/server-context.cpp:137 and :161
//     update_slots() starts from common_batch_clear(batch) and only adds rows
//     for slots that are processing at that moment.
//
// FAILURE SCENARIO
// A client cancels (or disconnects) while a long prompt is being prefilled in
// round-robin chunks — NUBatch tokens per pass, so an NBatch-sized tray needs
// several passes and the ctx.Err() check at batchgen_engine.go:374 is evaluated
// between them. Pass 1 pushes rows for seq N; the cancel lands; pass 2 releases
// the slot and clears seq N; fillSlots assigns a queued request to the same slot
// and pushes ITS rows for seq N; the decode commits both. The new request's
// sequence now contains cells from a different conversation at positions beyond
// its own prompt, and the model attends to them — which is exactly the
// "asserts something then contradicts what it already said" symptom. On an IMC
// hit the restored prefix is polluted the same way, because the restore also
// runs inside startSlot, before the decode.
//
// FIX: either compact the released slot's rows out of e.batch in finishSlot, or
// move fillSlots after the decode so a freed sequence can never be reused
// within the same batch.
func TestReleasedSlotRowsAreRemovedFromPendingBatch(t *testing.T) {
	const (
		chunkLimit = 4  // e.model.cfg.NUBatch(), batchgen_engine.go:352
		nBatch     = 64 // e.model.cfg.NBatch()
	)

	slots := []*mirrorGenSlot{
		{seq: 0, active: true, req: "reqA", prefill: 3 * chunkLimit, cancelled: true},
	}

	rows := mirrorAssembleBatch(slots, chunkLimit, nBatch, []string{"reqB"}, 1)

	if len(rows) == 0 {
		t.Fatal("mirror produced an empty batch; the assertion has drifted")
	}

	// Invariant: one llama_decode must never carry rows for the same KV
	// sequence on behalf of two different requests.
	perSeq := make(map[int]map[string][]int)
	for _, r := range rows {
		if perSeq[r.seq] == nil {
			perSeq[r.seq] = make(map[string][]int)
		}
		perSeq[r.seq][r.req] = append(perSeq[r.seq][r.req], r.pos)
	}

	for seq, byReq := range perSeq {
		if len(byReq) <= 1 {
			continue
		}

		var detail []string
		for req, positions := range byReq {
			detail = append(detail, req+" at positions "+fmtInts(positions))
		}
		slices.Sort(detail)

		t.Errorf("KV sequence %d carries rows for %d different requests in one llama.Decode: %s\n"+
			"batchgen_engine.go:373-383 releases a slot mid-assembly, batchgen_finish.go:176-182 "+
			"clears its KV sequence without removing its rows from e.batch, and "+
			"batchgen_engine.go:418 (fillSlots) hands the same slot/seqID to a queued request "+
			"before the decode at batchgen_engine.go:458. llama.cpp cannot do this: "+
			"server-queue.cpp:139-163 assigns every slot before update_slots() builds the batch.",
			seq, len(byReq), strings.Join(detail, "; "))
	}
}

// fmtInts renders a position list without pulling fmt into the hot assertion.
func fmtInts(xs []int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, x := range xs {
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteString(itoa(x))
	}
	b.WriteByte(']')

	return b.String()
}

func itoa(x int) string {
	if x == 0 {
		return "0"
	}

	neg := x < 0
	if neg {
		x = -x
	}

	var buf [20]byte
	i := len(buf)
	for x > 0 {
		i--
		buf[i] = byte('0' + x%10)
		x /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}

	return string(buf[i:])
}

// -----------------------------------------------------------------------------
// Finding 3: the token that reaches MaxTokens has its text thrown away.

// mirrorEmitTokenTail is a VERBATIM MIRROR of the tail of
// (*batchEngine).handleSampledToken, from the token counter to the content
// store:
//
//	sdk/kronk/model/batchgen_tokens.go:188-193   count the token
//	sdk/kronk/model/batchgen_tokens.go:205       outputTokens := s.reasonTokens + s.completionTokens
//	sdk/kronk/model/batchgen_tokens.go:207-210   if outputTokens >= MaxTokens { e.finishSlot(s, nil); return }
//	sdk/kronk/model/batchgen_tokens.go:213-222   store content in the final accumulator
//	sdk/kronk/model/batchgen_tokens.go:233       stream the delta
//
// The same ordering appears in the partial-UTF-8 branch at
// sdk/kronk/model/batchgen_tokens.go:135-155, which returns before the parser
// ever sees the bytes.
//
// Returns true when the slot was finished by this token.
//
// MAINTAINER: when batchgen_tokens.go:205-222 is reordered, update this mirror
// in the same commit.
func mirrorEmitTokenTail(completionTokens *int, maxTokens int, content string, final *strings.Builder) bool {
	*completionTokens++ // batchgen_tokens.go:192

	outputTokens := *completionTokens // batchgen_tokens.go:205

	if outputTokens >= maxTokens { // batchgen_tokens.go:207
		return true // e.finishSlot(s, nil); return
	}

	final.WriteString(content) // batchgen_tokens.go:221

	return false
}

// TestMaxTokensKeepsTheLimitReachingTokensText pins the one-token content loss
// at the max-tokens boundary on the plain path.
//
// FINDING
// handleSampledToken counts the token, then compares the running total against
// Params.MaxTokens and calls finishSlot BEFORE the token's text is written to
// finalContent/finalReasoning/finalTooling and before the streaming delta is
// sent. The token that trips the limit is counted in Usage but its text is
// never delivered to the caller by either transport.
//
// PRODUCTION LINE PINNED: sdk/kronk/model/batchgen_tokens.go:205-210 (and the
// identical partial-UTF-8 variant at :146-151).
//
// LLAMA.CPP REFERENCE (correct behaviour)
// .extras/llama.cpp/tools/server/server-context.cpp:1895-1899 —
// process_token() does slot.add_token(result) and send_partial_response() for
// the token FIRST, and only afterwards, at :1913-1918, evaluates
// `!slot.has_budget(params_base)` and sets STOP_TYPE_LIMIT. The limit-reaching
// token's text is always part of the response.
//
// FAILURE SCENARIO
// A request capped at N tokens returns the text of tokens 1..N-1 while
// reporting OutputTokens = N. Since Params.MaxTokens defaults to the whole
// context window, in practice this bites callers who set an explicit cap: the
// answer is silently truncated one token earlier than requested, and for a
// single-token final piece (a closing brace, a digit) the visible output is
// wrong rather than merely short.
//
// FIX: store and stream the content, then evaluate the budget — i.e. move the
// MaxTokens check below batchgen_tokens.go:238.
func TestMaxTokensKeepsTheLimitReachingTokensText(t *testing.T) {
	const maxTokens = 3

	pieces := []string{"alpha", "beta", "gamma"}

	var final strings.Builder
	completionTokens := 0
	emitted := 0

	for _, piece := range pieces {
		if mirrorEmitTokenTail(&completionTokens, maxTokens, piece, &final) {
			break
		}
		emitted++
	}

	want := strings.Join(pieces, "")

	if got := final.String(); got != want {
		t.Errorf("content after generating %d tokens with MaxTokens=%d = %q, want %q (%d of %d token texts delivered)\n"+
			"batchgen_tokens.go:207-210 evaluates `outputTokens >= s.job.params.MaxTokens` "+
			"and calls finishSlot BEFORE the content store at :213-222 and the delta at :233, "+
			"so the token that reaches the limit is counted in Usage but its text is dropped "+
			"from both the streamed deltas and the final response.\n"+
			"llama.cpp emits the token first (server-context.cpp:1895-1899) and only then "+
			"applies the budget check (server-context.cpp:1913-1918).",
			completionTokens, maxTokens, got, want, emitted, len(pieces))
	}
}

// -----------------------------------------------------------------------------
// Finding 4: nothing drains the parser state machine at end of generation.

// TestGenerationEndDrainsParserHeldBackContent pins the missing
// end-of-generation flush for content the parser state machine is holding.
//
// FINDING
// handleSampledToken routes every token's text through
// s.stateMachine.Classify (sdk/kronk/model/batchgen_tokens.go:158). Parser
// state machines legitimately withhold text across tokens while they wait for a
// closing marker — e.g. sdk/kronk/parsers/standard/state_machine.go:51-55
// buffers tool-call bytes into toolCallBuf until "</tool_call>" arrives, and
// sdk/kronk/parsers/qwen/state_machine.go:88-89 plus its pendingTagBuf
// lookahead (:47-68) do the same for Qwen, the lineage the reported failures
// were seen on.
//
// The model.StateMachine contract (sdk/kronk/model/parser.go:73-80) exposes only
// Classify and Reset. finishSlot flushes s.utf8Buf
// (sdk/kronk/model/batchgen_finish.go:261-278) but never asks the state machine
// for what it is still holding, and the deferred s.reset()
// (sdk/kronk/model/batchgen_finish.go:62 -> batchgen_slot.go:379-381) calls
// stateMachine.Reset(), which discards it.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/parser.go:73-80        StateMachine has no drain/flush
//   - sdk/kronk/model/batchgen_finish.go:261-278  only utf8Buf is flushed
//
// LLAMA.CPP REFERENCE (correct behaviour)
// .extras/llama.cpp/tools/server/server-context.cpp:1868-1898 —
// slot.generated_text is the single source of truth. Text withheld from the
// stream by find_partial_stop_string is not deleted; it stays in
// generated_text, and send_final_response ships whatever is left. Nothing the
// model produced can be lost by a stop-sequence lookahead that never resolves.
//
// FAILURE SCENARIO
// The model emits "<tool_call>" and the JSON body, then hits EOG or MaxTokens
// before "</tool_call>" (a routine outcome once a reasoning model has burned
// its budget). s.toolFlag > 0, so finishSlot takes the tool-call branch
// (sdk/kronk/model/batchgen_finish.go:282) — over an EMPTY s.finalTooling,
// because every buffered byte is still inside the state machine. The caller
// gets no tool call, no content, and no error: the task is started and then
// silently dropped. The Qwen pendingTagBuf path loses the trailing "<" the same
// way.
//
// FIX: add a drain step to the StateMachine contract (returning the held-back
// content and its channel) and call it from finishSlot before s.reset(), or
// mirror llama.cpp and keep the raw generated text on the slot as the
// authority.
func TestGenerationEndDrainsParserHeldBackContent(t *testing.T) {
	// (a) Does the contract let the engine ask for held-back content?
	iface := reflect.TypeOf((*StateMachine)(nil)).Elem()

	var methods []string
	drainable := false
	for i := range iface.NumMethod() {
		name := iface.Method(i).Name
		methods = append(methods, name)

		switch name {
		case "Drain", "Flush", "Finish", "Pending", "Remaining", "Close":
			drainable = true
		}
	}

	// (b) Or does finishSlot reach into the state machine itself?
	fset, files := parseModelPackage(t)

	finishSlotReadsStateMachine := false
	for _, f := range files {
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil || fd.Name.Name != "finishSlot" {
				continue
			}

			ast.Inspect(fd.Body, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if ok && sel.Sel.Name == "stateMachine" {
					finishSlotReadsStateMachine = true
					t.Logf("finishSlot touches stateMachine at %s", posOf(fset, sel))
				}

				return true
			})
		}
	}

	if drainable || finishSlotReadsStateMachine {
		return
	}

	t.Errorf("nothing drains the parser state machine at end of generation: StateMachine exposes only %s and finishSlot never touches s.stateMachine\n"+
		"Parser state machines withhold text across tokens while they wait for a closing "+
		"marker (sdk/kronk/parsers/standard/state_machine.go:51-55, "+
		"sdk/kronk/parsers/qwen/state_machine.go:47-68 and :88-89). finishSlot flushes only "+
		"s.utf8Buf (sdk/kronk/model/batchgen_finish.go:261-278) and its deferred s.reset() "+
		"calls stateMachine.Reset() (sdk/kronk/model/batchgen_slot.go:379-381), so an "+
		"unterminated <tool_call> at EOG or MaxTokens is discarded whole: the tool-call "+
		"branch at batchgen_finish.go:282 runs over an empty s.finalTooling and the caller "+
		"gets an empty response with no error.\n"+
		"llama.cpp cannot lose it — slot.generated_text keeps every byte and "+
		"send_final_response ships the remainder (.extras/llama.cpp/tools/server/"+
		"server-context.cpp:1868-1898).",
		strings.Join(methods, ", "))
}

// -----------------------------------------------------------------------------
// Finding 5: the M-RoPE generation step applies grammar during reasoning but
// never accepts it.

// TestMRoPEGenerationGrammarIsGatedOnReasonFlag pins the grammar-gating
// asymmetry on the plain M-RoPE generation step.
//
// FINDING
// Kronk applies grammar constraints only outside the reasoning phase, because
// masking thinking tokens "would corrupt the thinking tokens and prevent the
// model from closing the think block"
// (sdk/kronk/model/batchgen_tokens.go:15-18). Two of the three plain-path
// sampling sites honour that:
//
//	sdk/kronk/model/batchgen_tokens.go:21   case s.grammarSampler != nil && s.reasonFlag == 0:
//	sdk/kronk/model/batchgen_tokens.go:261  case s.grammarSampler != nil && s.reasonFlag == 0:
//
// and the matching grammar Accept is gated identically at
// sdk/kronk/model/batchgen_tokens.go:59. The M-RoPE generation step is not:
//
//	sdk/kronk/model/batchgen_engine.go:256  case s.grammarSampler != nil:
//
// PRODUCTION LINE PINNED: sdk/kronk/model/batchgen_engine.go:255-260, inside
// the `if s.useMRoPE` generation branch of processBatch.
//
// LLAMA.CPP REFERENCE (correct behaviour)
// .extras/llama.cpp/common/sampling.cpp:630-643 — common_sampler_sample applies
// the grammar only when grammar_should_apply(gsmpl) is true, and
// common_sampler_accept advances the grammar under the SAME predicate. Apply and
// accept are never allowed to disagree about whether the grammar is live.
//
// FAILURE SCENARIO
// A vision/M-RoPE request with a JSON schema on a reasoning model: every
// reasoning token is sampled from grammar-masked logits (grammar.go:419-425
// writes the -inf mask straight back into the context's logit row), so the
// thinking text degenerates into schema-shaped fragments and the model may
// never be able to emit "</think>" — it starts the task and never finishes it.
// Meanwhile batchgen_tokens.go:59 skips the Accept, so the grammar stack never
// advances: once reasoning ends, the answer is constrained from a stale root
// state that does not match the tokens already sampled.
//
// FIX: add `&& s.reasonFlag == 0` at batchgen_engine.go:256 so the M-RoPE
// generation step matches processSlotToken and sampleFirstToken.
func TestMRoPEGenerationGrammarIsGatedOnReasonFlag(t *testing.T) {
	root := kronkRepoRoot(t)
	path := root + "/sdk/kronk/model/batchgen_engine.go"

	fset := token.NewFileSet()
	f := parseKronkSource(t, fset, path)
	fd := findKronkFunc(t, f, path, "processBatch")

	mentions := func(n ast.Node, name string) bool {
		found := false
		ast.Inspect(n, func(x ast.Node) bool {
			if id, ok := x.(*ast.Ident); ok && id.Name == name {
				found = true
			}
			return true
		})

		return found
	}

	checked := 0

	ast.Inspect(fd.Body, func(n ast.Node) bool {
		ifStmt, ok := n.(*ast.IfStmt)
		if !ok || ifStmt.Cond == nil || !mentions(ifStmt.Cond, "useMRoPE") {
			return true
		}

		ast.Inspect(ifStmt.Body, func(x ast.Node) bool {
			clause, ok := x.(*ast.CaseClause)
			if !ok || len(clause.List) == 0 {
				return true
			}

			for _, cond := range clause.List {
				if !mentions(cond, "grammarSampler") {
					continue
				}

				checked++

				if !mentions(cond, "reasonFlag") {
					t.Errorf("%s: M-RoPE generation grammar case does not test s.reasonFlag\n"+
						"batchgen_tokens.go:21 and :261 both use `s.grammarSampler != nil && s.reasonFlag == 0`, "+
						"and the grammar Accept at batchgen_tokens.go:59 is gated the same way. This site "+
						"applies the grammar mask to reasoning tokens and then never accepts them, so the "+
						"grammar stack desynchronises from the sampled tokens.\n"+
						"llama.cpp keeps apply and accept under one predicate "+
						"(grammar_should_apply, .extras/llama.cpp/common/sampling.cpp:630-643).",
						posOf(fset, cond))
				}
			}

			return true
		})

		return true
	})

	if checked == 0 {
		t.Fatalf("%s: no grammarSampler case found inside the `if s.useMRoPE` generation "+
			"branch of processBatch; this assertion has drifted from the source", path)
	}
}
