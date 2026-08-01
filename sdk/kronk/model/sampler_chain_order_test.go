package model

import (
	"fmt"
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// =============================================================================
// Sampler-chain construction, parameter sentinels and penalty accounting.
//
// The reference implementation for every assertion in this file is llama.cpp
// b10211 as vendored under .extras/llama.cpp:
//
//   - common/sampling.cpp:325-381  common_sampler_init, the canonical chain
//   - common/common.h:224-269      common_params_sampling defaults + sentinels
//   - tools/server/server-schema.cpp:89-155, 550-560  JSON param semantics
//   - tools/server/server-context.cpp:375-393  init_sampler (prompt seeding)
//
// Where a defect lives in pure parameter resolution the real Kronk function is
// called directly (m.parseParams / m.adjustParams need only m.cfg and m.log,
// see newTempTestModel in params_temperature_test.go). Where the defect lives
// inside toSampler — which calls into the yzma FFI and therefore needs a loaded
// shared library — the assertion is made over the checked-in source, the same
// technique doc_comment_claims_test.go and mtp_ctxparams_source_test.go use.
// The source assertions locate code by declaration name, never by line number.
// =============================================================================

// TestParseParamsDryPenaltyLastKeepsDryEnabled pins the DRY sentinel defect.
//
// FINDING
// DRY is dead code in Kronk unless the caller explicitly passes a POSITIVE
// dry_penalty_last_n. Both "not supplied" and the llama.cpp sentinel -1
// resolve to 0, and 0 means "DRY disabled" one layer down.
//
// PRODUCTION
//
//	sdk/kronk/model/params.go:701-703
//		if p.DryPenaltyLast < 0 {
//			p.DryPenaltyLast = DefDryPenaltyLast   // params.go:37 -> 0
//		}
//
//	sdk/kronk/model/params.go:790-791 hands that value to
//	llama.SamplerInitDry(..., p.DryPenaltyLast, nil) whenever
//	p.DryMultiplier > 0.
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/common/common.h:245
//		int32_t dry_penalty_last_n = -1; // 0 = disable penalty, -1 = context size
//	.extras/llama.cpp/tools/server/server-schema.cpp:151-155 + 557-559
//		"How many tokens to scan for repetitions (0 = disabled, -1 = context
//		size)"; a request value of -1 is rewritten to n_ctx_slot, never to 0.
//	.extras/llama.cpp/src/llama-sampler.cpp:3200-3205
//		effective_dry_penalty_last_n = (dry_penalty_last_n == -1) ? n_ctx_train
//		                                                         : max(v, 0);
//		dry_enabled = (dry_multiplier != 0 && dry_base >= 1 &&
//		               dry_penalty_last_n != 0);
//	.extras/llama.cpp/src/llama-sampler.cpp:2939, 2950 — apply() returns
//	immediately when dry_penalty_last_n == 0.
//
// FAILURE SCENARIO
// A long reasoning conversation starts to loop, so the caller enables the
// anti-loop sampler: {"dry_multiplier": 1.0}. Kronk logs the DRY sampler as
// active ("sampler-chain ... sampler=dry multiplier=1.00 penalty_last_n=0",
// params.go:792) and adds it to the chain, but the sampler was constructed with
// a zero-length ring buffer and never penalizes anything. The same request
// against llama.cpp's server gets a full-context DRY window. Passing the
// documented llama.cpp sentinel -1 explicitly does not help either: it is
// rewritten to 0 by params.go:701-703 before it reaches the sampler.
func TestParseParamsDryPenaltyLastKeepsDryEnabled(t *testing.T) {
	m := newTempTestModel()
	ctxWindow := int32(m.cfg.ContextWindow())

	tests := []struct {
		name string
		doc  D
	}{
		{name: "dry enabled, dry_penalty_last_n not supplied", doc: D{"dry_multiplier": 1.0}},
		{name: "dry enabled, dry_penalty_last_n -1 means context size", doc: D{"dry_multiplier": 1.0, "dry_penalty_last_n": -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fmt.Sprintf("%#v", tt.doc)

			p, err := m.parseParams(tt.doc)
			if err != nil {
				t.Fatalf("parseParams(%s): unexpected error: %v", input, err)
			}

			if p.DryMultiplier <= 0 {
				t.Fatalf("precondition: parseParams(%s) resolved DryMultiplier = %v, want > 0", input, p.DryMultiplier)
			}

			// llama.cpp accepts exactly two shapes here: the -1 sentinel
			// (resolved to the slot context size by the server) or a positive
			// window. 0 is reserved for "DRY off".
			switch {
			case p.DryPenaltyLast == -1, p.DryPenaltyLast > 0:
				// DRY can actually do work.
			default:
				t.Errorf("parseParams(%s): resolved DryPenaltyLast = %d, want -1 (context size) or > 0\n"+
					"params.go:701-703 collapses both \"unset\" and the llama.cpp -1 sentinel to\n"+
					"DefDryPenaltyLast (params.go:37 = 0). llama-sampler.cpp:3205 treats\n"+
					"dry_penalty_last_n == 0 as \"DRY disabled\" and builds a zero-length token\n"+
					"ring buffer, so the DRY sampler added at params.go:791 is a no-op even\n"+
					"though params.go:792 logs it as active. llama.cpp's default is -1 =\n"+
					"context size (common.h:245, server-schema.cpp:557-559); this model's\n"+
					"context window is %d.",
					input, p.DryPenaltyLast, ctxWindow)
			}
		})
	}
}

// TestSamplingDefaultsHonorGGUFMetadata pins the ignored per-model sampling
// metadata.
//
// FINDING
// llama.cpp seeds its sampling defaults from the GGUF's general.sampling.*
// keys whenever the user did not explicitly override them. Kronk reads the
// whole metadata table but never consults those keys, so every model gets
// Kronk's hardcoded defaults instead of the ones it shipped with.
//
// PRODUCTION
//
//	sdk/kronk/model/models.go:119-146  toModelInfo copies every GGUF key/value
//	    into ModelInfo.Metadata — the general.sampling.* values are already in
//	    memory.
//	sdk/kronk/model/params.go:688-760  adjustParams resolves unset fields from
//	    the package constants only: DefTemp 0.8 (params.go:96), DefTopK 40
//	    (params.go:101), DefTopP 0.9 (params.go:112), DefMinP 0 (params.go:67).
//	    No branch anywhere in the repo reads a "general.sampling." key.
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/common/common.cpp:1142-1198  common_init_sampler_from_model
//	    get_int32(... SAMPLING_TOP_K, sparams.top_k, ...CONFIG_TOP_K);
//	    get_float(... SAMPLING_TOP_P, sparams.top_p, ...CONFIG_TOP_P);
//	    get_float(... SAMPLING_TEMP,  sparams.temp,  ...CONFIG_TEMP);
//	    (plus min_p, xtc_*, penalty_last_n, penalty_repeat, mirostat_*, and the
//	    whole sampler SEQUENCE)
//	.extras/llama.cpp/common/common.cpp:1266  called from common_init_result,
//	    i.e. on every model load, before any sampler is built. The
//	    user_sampling_config bitfield (common/common.h:256, common/arg.cpp:1902+)
//	    ensures an explicit CLI/request value still wins.
//	.extras/llama.cpp/src/llama-arch.cpp:156-167 / src/llama-model.cpp:2671-2676
//	    define the key names.
//
// FAILURE SCENARIO — measured on the user's actual model
// /Users/florin/.kronk/models/unsloth/Qwen3.6-35B-A3B-MTP-GGUF/
// mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf carries:
//
//	general.architecture       = qwen35moe
//	qwen35moe.context_length   = 262144
//	qwen35moe.nextn_predict_layers = 1     (MTP)
//	general.sampling.top_k     = 20
//	general.sampling.top_p     = 0.95
//	general.sampling.temp      = 1
//
// llama.cpp therefore serves this model at temp 1.0 / top_k 20 / top_p 0.95.
// Kronk serves it at temp 0.8 / top_k 40 / top_p 0.9: a lower temperature on a
// wider-but-nucleus-clipped candidate set. That combination is the classic
// repetition-loop regime for Qwen reasoning models — the model's own card
// warns against low temperature with thinking enabled — and it is exactly the
// reported symptom (long reasoning turns that loop or double back and
// contradict themselves). Nothing in the request has to be wrong for this to
// happen: it is the default path.
func TestSamplingDefaultsHonorGGUFMetadata(t *testing.T) {
	root := kronkRepoRoot(t)
	dir := filepath.Join(root, "sdk", "kronk", "model")

	// Ground the comparison in the current constants rather than restating
	// them: these are what a Qwen3.6 request gets today.
	kronkDefaults := fmt.Sprintf("DefTemp=%v DefTopK=%d DefTopP=%v DefMinP=%v", float32(DefTemp), DefTopK, float32(DefTopP), float32(DefMinP))

	found := false
	for _, path := range nonTestGoFiles(t, dir) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}

		if strings.Contains(string(src), "general.sampling.") {
			found = true
			break
		}
	}

	if !found {
		t.Errorf("no source under %s reads any GGUF \"general.sampling.*\" key\n"+
			"kronk defaults in force: %s\n"+
			"the MTP model under test ships general.sampling.top_k=20,\n"+
			"general.sampling.top_p=0.95, general.sampling.temp=1 (verified with\n"+
			"gguf metadata of mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf), and\n"+
			"toModelInfo (models.go:119-146) already loads those values into\n"+
			"ModelInfo.Metadata. llama.cpp applies them to the sampling params on\n"+
			"every model load unless the caller overrode the field\n"+
			"(common/common.cpp:1142-1198, called at :1266), so llama.cpp runs this\n"+
			"model at temp 1.0/top_k 20/top_p 0.95 while Kronk runs it at\n"+
			"temp 0.8/top_k 40/top_p 0.9.",
			dir, kronkDefaults)
	}
}

// TestParseParamsRepeatLastNSentinels pins the repeat_last_n sentinel defect.
//
// FINDING
// repeat_last_n loses both of its llama.cpp sentinels. -1 ("penalize over the
// whole context") and 0 ("penalties off") are both rewritten to 64.
//
// PRODUCTION
//
//	sdk/kronk/model/params.go:717-719
//		if p.RepeatLastN <= 0 {
//			p.RepeatLastN = DefRepeatLastN   // params.go:80 -> 64
//		}
//
//	The resolved value is passed to llama.SamplerInitPenalties at
//	sdk/kronk/model/params.go:785, which is added to the chain whenever any of
//	repeat/frequency/presence penalty is non-default.
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/common/common.h:238
//		int32_t penalty_last_n = 64; // last n tokens to penalize
//		                             // (0 = disable penalty, -1 = context size)
//	.extras/llama.cpp/tools/server/server-schema.cpp:126-128 (hard limits
//	-1..INT32_MAX, "0 = disabled, -1 = ctx-size") and :552-555, which rewrites
//	a requested -1 to the slot context size — never to 64.
//	.extras/llama.cpp/src/llama-sampler.cpp:2762-2782 sizes the penalty ring
//	buffer from penalty_last_n and short-circuits when it is 0.
//
// FAILURE SCENARIO
// Two symmetric mis-resolutions, both silent:
//
//   - {"repeat_penalty": 1.15, "repeat_last_n": -1} asks for a penalty across
//     the whole conversation (4096 tokens here). Kronk penalizes the last 64
//     tokens instead — a ~64x narrower window than the same request would get
//     from llama.cpp's server.
//   - {"repeat_penalty": 1.15, "repeat_last_n": 0} asks for the penalty to be
//     switched off. Kronk switches it ON over 64 tokens. In a long reasoning
//     turn a 64-token repeat penalty suppresses exactly the tokens that hold a
//     chain of thought together (identifiers, "therefore", quoted values),
//     which is one of the mechanisms behind self-contradiction.
func TestParseParamsRepeatLastNSentinels(t *testing.T) {
	m := newTempTestModel()
	ctxWindow := int32(m.cfg.ContextWindow())

	t.Run("-1 means context size", func(t *testing.T) {
		doc := D{"repeat_penalty": 1.15, "repeat_last_n": -1}
		input := fmt.Sprintf("%#v", doc)

		p, err := m.parseParams(doc)
		if err != nil {
			t.Fatalf("parseParams(%s): unexpected error: %v", input, err)
		}

		if p.RepeatLastN != -1 && p.RepeatLastN != ctxWindow {
			t.Errorf("parseParams(%s): resolved RepeatLastN = %d, want -1 or the context window %d\n"+
				"params.go:717-719 rewrites every value <= 0 to DefRepeatLastN (64), so the\n"+
				"llama.cpp -1 = context-size sentinel (common.h:238,\n"+
				"server-schema.cpp:552-555) silently becomes a 64-token window.",
				input, p.RepeatLastN, ctxWindow)
		}
	})

	t.Run("0 disables the penalty", func(t *testing.T) {
		doc := D{"repeat_penalty": 1.15, "repeat_last_n": 0}
		input := fmt.Sprintf("%#v", doc)

		p, err := m.parseParams(doc)
		if err != nil {
			t.Fatalf("parseParams(%s): unexpected error: %v", input, err)
		}

		if p.RepeatLastN != 0 {
			t.Errorf("parseParams(%s): resolved RepeatLastN = %d, want 0 (penalty disabled)\n"+
				"params.go:717-719 cannot distinguish an explicit 0 from an absent field, so\n"+
				"a caller asking llama.cpp's \"0 = disabled\" (common.h:238,\n"+
				"server-schema.cpp:126-128) gets a 64-token repetition penalty applied to\n"+
				"their request instead.",
				input, p.RepeatLastN)
		}
	})
}

// TestParseParamsTopKDisableSentinel pins the top_k sentinel defect.
//
// FINDING
// top_k cannot be disabled. llama.cpp treats top_k <= 0 as "use the vocab
// size", i.e. no truncation; Kronk rewrites it to 40.
//
// PRODUCTION
//
//	sdk/kronk/model/params.go:733-735
//		if p.TopK <= 0 {
//			p.TopK = DefTopK   // params.go:101 -> 40
//		}
//
//	The value reaches llama.SamplerInitTopK at sdk/kronk/model/params.go:796,
//	unconditionally, and is also handed to the speculative verification filter
//	(batchgen_speculative.go:498, :543, :599 -> logprobs.go:129).
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/common/common.h:229
//		int32_t top_k = 40; // <= 0 to use vocab size
//	.extras/llama.cpp/tools/server/server-schema.cpp:89-91
//		field_num("top_k", ...)->set_limits(0, INT32_MAX)
//		"Limit the next token selection to the K most probable tokens
//		 (0 = disabled)"
//
// FAILURE SCENARIO
// An OpenAI-compatible client that sends {"top_k": 0} to mean "no top-k, let
// min_p/top_p decide" gets a hard 40-token truncation. On a 35B MoE with a
// large vocab this clips the tail that min_p was tuned against, and the
// truncation is invisible: the sampler-chain log line (params.go:797) reports
// k=40 as if the caller had asked for it.
func TestParseParamsTopKDisableSentinel(t *testing.T) {
	m := newTempTestModel()

	tests := []struct {
		name string
		doc  D
	}{
		{name: "explicit zero disables top-k", doc: D{"top_k": 0}},
		{name: "negative disables top-k", doc: D{"top_k": -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := fmt.Sprintf("%#v", tt.doc)

			p, err := m.parseParams(tt.doc)
			if err != nil {
				t.Fatalf("parseParams(%s): unexpected error: %v", input, err)
			}

			if p.TopK > 0 {
				t.Errorf("parseParams(%s): resolved TopK = %d, want <= 0 (top-k disabled)\n"+
					"params.go:733-735 rewrites every top_k <= 0 to DefTopK (40). llama.cpp\n"+
					"documents top_k <= 0 as \"use vocab size\" (common.h:229) and its server\n"+
					"accepts 0 as \"disabled\" (server-schema.cpp:89-91), so top-k truncation\n"+
					"cannot be turned off through Kronk's public API.",
					input, p.TopK)
			}
		})
	}
}

// TestToSamplerAdaptivePIsTheFinalSelector pins two chain-construction defects
// around the adaptive-p sampler.
//
// FINDING
// (a) adaptive-p is inserted BEFORE the temperature sampler, and
// (b) a dist sampler is appended afterwards regardless.
// adaptive-p is a selecting sampler: it rewrites every logit into a
// distance-from-target curve and picks a token. Running temp_ext + dist after
// it both discards its selection and samples from the rewritten curve.
//
// PRODUCTION
//
//	sdk/kronk/model/params.go:813-816   adaptive-p (guarded by AdaptivePTarget > 0)
//	sdk/kronk/model/params.go:819-821   temp_ext  (unconditional)
//	sdk/kronk/model/params.go:823-825   dist      (unconditional)
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/common/sampling.cpp:363-381
//		case COMMON_SAMPLER_TYPE_ADAPTIVE_P:
//		    // ... selects a single token, so we will add `dist` at the end of
//		    // the chain by default, unless the user specifically included
//		    // `adaptive-p`.
//		    use_adaptive_p = true;
//		...
//		if (use_adaptive_p) {
//		    samplers.push_back(llama_sampler_init_adaptive_p(...));
//		} else {
//		    samplers.push_back(llama_sampler_init_dist(params.seed));
//		}
//
//	adaptive-p is therefore pushed AFTER the whole params.samplers loop (whose
//	default order ends with COMMON_SAMPLER_TYPE_TEMPERATURE, common.h:260-269)
//	and REPLACES dist.
//
//	.extras/llama.cpp/src/llama-sampler.cpp:3311-3371 shows why the position
//	matters: adaptive_p_apply() overwrites cur_p->data[i].logit with
//	PEAK_LOGIT_VALUE - SHARPNESS*dist^2/(1+dist), sets cur_p->selected, and
//	stores pending_token_id. adaptive_p_accept() only updates its EMA when the
//	accepted token equals pending_token_id.
//
// FAILURE SCENARIO
// A request with {"adaptive_p_target": 0.1}: adaptive-p replaces the real
// logits with the peak-shaped curve and selects a token; temp_ext then rescales
// that synthetic curve and dist re-samples from it, so the emitted token is
// almost never adaptive-p's pick. Because it differs, adaptive_p_accept()
// (llama-sampler.cpp:3362-3373) never advances its EMA, so the adaptive target
// never adapts. The result is neither adaptive-p sampling nor ordinary
// sampling: the token comes from a distribution that has no relation to the
// model's logits.
func TestToSamplerAdaptivePIsTheFinalSelector(t *testing.T) {
	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "model", "params.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)
	fd := findKronkFunc(t, file, path, "toSampler")

	src, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	adds := samplerChainAdds(fset, root, src, fd)
	if len(adds) == 0 {
		t.Fatalf("no llama.SamplerInit* calls found in toSampler (%s): re-point this test", path)
	}

	t.Run("adaptive-p comes after temperature", func(t *testing.T) {
		adaptive := findSamplerAdd(adds, "SamplerInitAdaptiveP")
		temp := findSamplerAdd(adds, "SamplerInitTempExt")

		if adaptive == nil || temp == nil {
			t.Skipf("toSampler no longer adds both adaptive-p and temp_ext (adaptive=%v temp=%v)", adaptive != nil, temp != nil)
		}

		if adaptive.order < temp.order {
			t.Errorf("toSampler adds adaptive-p (%s, chain position %d) BEFORE temp_ext (%s, chain position %d)\n"+
				"chain as constructed: %s\n"+
				"llama.cpp pushes adaptive-p after the whole sampler loop, i.e. after\n"+
				"temperature (common/sampling.cpp:373-381, defaults common.h:260-269).\n"+
				"adaptive-p is a SELECTING sampler that overwrites every logit with a\n"+
				"distance-from-target curve (src/llama-sampler.cpp:3311-3360); running\n"+
				"temperature on that synthetic curve is meaningless.",
				adaptive.pos, adaptive.order, temp.pos, temp.order, chainSummary(adds))
		}
	})

	t.Run("dist is not added alongside adaptive-p", func(t *testing.T) {
		adaptive := findSamplerAdd(adds, "SamplerInitAdaptiveP")
		dist := findSamplerAdd(adds, "SamplerInitDist")

		if adaptive == nil || dist == nil {
			t.Skipf("toSampler no longer adds both adaptive-p and dist (adaptive=%v dist=%v)", adaptive != nil, dist != nil)
		}

		// llama.cpp makes the two mutually exclusive. In Go that has to show
		// up as a guard on the dist call that mentions the adaptive-p switch.
		guarded := false
		for _, cond := range dist.conds {
			if strings.Contains(cond, "AdaptiveP") {
				guarded = true
				break
			}
		}

		if !guarded {
			t.Errorf("toSampler adds a dist sampler (%s) that is not mutually exclusive with adaptive-p (%s)\n"+
				"guards on the dist call: %v\n"+
				"chain as constructed: %s\n"+
				"common/sampling.cpp:373-381 adds EITHER adaptive-p OR dist, never both,\n"+
				"because each one selects the final token. With both present, dist\n"+
				"re-selects from adaptive-p's rewritten logits, so adaptive-p's choice is\n"+
				"discarded and its EMA never updates (src/llama-sampler.cpp:3362-3373).",
				dist.pos, adaptive.pos, dist.conds, chainSummary(adds))
		}
	})
}

// TestToSamplerDryUsesDefaultSequenceBreakers pins the missing DRY sequence
// breakers.
//
// FINDING
// Kronk constructs the DRY sampler with a nil sequence-breaker list, so DRY
// matches n-grams straight across newlines, quotes and colons.
//
// PRODUCTION
//
//	sdk/kronk/model/params.go:791
//		llama.SamplerChainAdd(sampler, llama.SamplerInitDry(m.vocab,
//			int32(m.cfg.ContextWindow()), p.DryMultiplier, p.DryBase,
//			p.DryAllowedLen, p.DryPenaltyLast, nil))
//
//	yzma passes a null pointer and num_breakers = 0 for a nil slice
//	(.extras/yzma/pkg/llama/sampling.go:418-438).
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/common/common.h:257
//		std::vector<std::string> dry_sequence_breakers = {"\n", ":", "\"", "*"};
//	.extras/llama.cpp/common/sampling.cpp:329-339 forwards those breakers into
//	llama_sampler_init_dry; tools/server/server-task.cpp:115 exposes them as a
//	per-request field with the same default.
//
// FAILURE SCENARIO
// With DRY enabled, restarts that legitimately repeat across a boundary — a
// bulleted list, a repeated JSON key, a quoted identifier echoed from the
// prompt — are treated as one long repeated n-gram, because nothing resets the
// match at "\n", ":" or '"'. DRY's penalty grows as base^(len-allowed_length),
// so a long cross-line match drives the penalty on structurally required
// tokens to the point where the model substitutes something else, which is the
// classic "starts fine, then derails / contradicts itself" failure. llama.cpp
// never runs DRY without breakers.
func TestToSamplerDryUsesDefaultSequenceBreakers(t *testing.T) {
	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "model", "params.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)
	fd := findKronkFunc(t, file, path, "toSampler")

	var call *ast.CallExpr
	ast.Inspect(fd, func(n ast.Node) bool {
		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if samplerInitName(ce) == "SamplerInitDry" {
			call = ce
		}

		return true
	})

	if call == nil {
		t.Skipf("toSampler no longer calls llama.SamplerInitDry (%s)", path)
	}

	if len(call.Args) == 0 {
		t.Fatalf("llama.SamplerInitDry called with no arguments at %s", srcPos(fset, root, call.Pos()))
	}

	last := call.Args[len(call.Args)-1]
	if id, ok := last.(*ast.Ident); ok && id.Name == "nil" {
		t.Errorf("%s: llama.SamplerInitDry is called with a nil sequence-breaker list\n"+
			"llama.cpp always passes dry_sequence_breakers, whose default is\n"+
			"{\"\\n\", \":\", \"\\\"\", \"*\"} (common/common.h:257, forwarded at\n"+
			"common/sampling.cpp:329-339). Without breakers the DRY sampler matches\n"+
			"repeated n-grams across line, quote and colon boundaries and penalizes\n"+
			"structurally required tokens.",
			srcPos(fset, root, last.Pos()))
	}
}

// TestSamplerPenaltyHistoryIsSeededWithPrompt pins the missing prompt seeding
// of the penalty / DRY history.
//
// FINDING
// Kronk only ever calls llama.SamplerAccept for tokens it generated
// (batchgen_tokens.go:63, reached from handleSampledToken). The prompt is never
// fed to the sampler chain, so the penalties and DRY ring buffers start empty
// on every request.
//
// PRODUCTION
//
//	sdk/kronk/model/batchgen_slot_start.go:96-97 creates the per-request chain
//	(m.toSampler) and nothing afterwards seeds it; the prefill path decodes the
//	prompt without touching s.sampler.
//	sdk/kronk/model/batchgen_tokens.go:63 is the only accept on s.sampler.
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/tools/server/server-context.cpp:375-393
//		void init_sampler() const {
//		    common_sampler_reset(smpl.get());
//		    ...
//		    for (int i = 0; i < (int) prompt.tokens.size(); i++) {
//		        const llama_token id = prompt.tokens[i];
//		        if (id != LLAMA_TOKEN_NULL) {
//		            common_sampler_accept(smpl.get(), id, false);
//		        }
//		    }
//		}
//
//	called from server-context.cpp:3571 whenever a slot's prompt is (re)made,
//	and common_sampler_accept forwards to llama_sampler_accept on the whole
//	chain (common/sampling.cpp:524-527, 552-586).
//
// FAILURE SCENARIO
// In a multi-turn reasoning conversation the prompt contains every previous
// turn. llama.cpp's penalty and DRY windows therefore span the conversation,
// while Kronk's span only the tokens generated in the current turn. Identical
// requests with identical repeat_penalty / dry_multiplier produce materially
// different distributions, and the first ~64 generated tokens of every turn are
// effectively penalty-free in Kronk — the model is free to restate (and then
// contradict) whatever it just said in the previous turn.
func TestSamplerPenaltyHistoryIsSeededWithPrompt(t *testing.T) {
	root := kronkRepoRoot(t)
	dir := filepath.Join(root, "sdk", "kronk", "model")

	// A fix may live in slot start, in the prefill path, or in a dedicated
	// seeding helper; accept any of those.
	seedingFunc := regexp.MustCompile(`(?i)(prefill|startslot|initsampler|seedsampler|seedpenalt|acceptprompt)`)

	var sites []string
	seeded := false

	for _, path := range nonTestGoFiles(t, dir) {
		fset := token.NewFileSet()
		file := parseKronkSource(t, fset, path)

		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}

			ast.Inspect(fd.Body, func(n ast.Node) bool {
				ce, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}

				sel, ok := ce.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "SamplerAccept" {
					return true
				}

				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "llama" {
					return true
				}

				sites = append(sites, fmt.Sprintf("%s in %s", srcPos(fset, root, ce.Pos()), fd.Name.Name))
				if seedingFunc.MatchString(fd.Name.Name) {
					seeded = true
				}

				return true
			})
		}
	}

	if len(sites) == 0 {
		t.Fatalf("no llama.SamplerAccept call sites found under %s: re-point this test", dir)
	}

	if !seeded {
		t.Errorf("no llama.SamplerAccept call seeds the prompt into the per-request sampler chain\n"+
			"call sites found: %v\n"+
			"llama.cpp's server accepts EVERY prompt token into the chain before\n"+
			"generation starts (tools/server/server-context.cpp:375-393, called from\n"+
			":3571), which is what fills the penalties ring buffer\n"+
			"(src/llama-sampler.cpp:2762-2782) and the DRY token history. Kronk creates\n"+
			"the chain at batchgen_slot_start.go:96-97 and only ever accepts generated\n"+
			"tokens (batchgen_tokens.go:63), so repeat/frequency/presence penalties and\n"+
			"DRY see none of the conversation history carried in the prompt.",
			sites)
	}
}

// TestSpeculativeVerifyTargetProbsIncludePenalties pins the divergence between
// the real sampler chain and the hand-rolled filter used to verify draft
// tokens.
//
// FINDING
// Speculative / MTP verification builds its target distribution with
// applySamplerFilters, which implements only suppress-bias -> top-k -> top-p ->
// min-p -> temperature. Penalties, DRY and XTC — every stateful sampler in the
// real chain — are absent, so p_target used for accept/reject (and for the
// rejection-adjusted resample) comes from a different distribution than the
// non-speculative path would produce.
//
// PRODUCTION
//
//	sdk/kronk/model/logprobs.go:129
//		func applySamplerFilters(logits, probs []float32,
//			suppressTokens []llama.Token, temperature, topP, minP float32,
//			topK int32, indices []int, fs *filterState) []int
//
//	callers: sdk/kronk/model/batchgen_speculative.go:498 (sparse verify),
//	:543 (full-vocab verify), :599 (all-accepted bonus sample).
//	The real chain is sdk/kronk/model/params.go:783-825 and does include
//	Penalties (:785), DRY (:791) and XTC (:809).
//
// LLAMA.CPP REFERENCE
//
//	.extras/llama.cpp/tools/server/server-context.cpp:3862
//		auto accepted = common_sampler_sample_and_accept_n(slot.smpl.get(),
//			slot.ctx_tgt, slot.spec_i_batch, slot.spec_draft);
//	.extras/llama.cpp/common/sampling.cpp:646-674 — that helper calls
//	common_sampler_sample + common_sampler_accept for EVERY draft position, i.e.
//	the identical full chain (penalties, DRY, XTC, temperature, dist) that
//	non-speculative decoding uses.
//
// FAILURE SCENARIO
// With repeat_penalty or DRY enabled and MTP on, a repeated token that the real
// chain would have penalized still has a high p_target in the verification
// filter, so ratio = p_target/q_draft >= 1 and the draft token is accepted
// unconditionally. Speculative decoding's whole correctness argument is that
// the accepted stream is distributed identically to the non-speculative stream;
// here the two paths use different distributions, so enabling MTP silently
// turns off repetition control and makes loops more likely, not less.
//
// The assertion is over the signature because the omission IS the signature:
// applySamplerFilters has no parameter through which penalty state could be
// expressed. If the fix routes verification through the real chain and deletes
// the helper, the test passes.
func TestSpeculativeVerifyTargetProbsIncludePenalties(t *testing.T) {
	root := kronkRepoRoot(t)
	path := filepath.Join(root, "sdk", "kronk", "model", "logprobs.go")

	fset := token.NewFileSet()
	file := parseKronkSource(t, fset, path)

	var fd *ast.FuncDecl
	for _, decl := range file.Decls {
		d, ok := decl.(*ast.FuncDecl)
		if ok && d.Name.Name == "applySamplerFilters" {
			fd = d
			break
		}
	}

	if fd == nil {
		t.Skip("applySamplerFilters is gone: speculative verification no longer uses a hand-rolled filter")
	}

	// Confirm the real chain does apply penalties, so the comparison is
	// grounded in the current source rather than restated from memory.
	paramsPath := filepath.Join(root, "sdk", "kronk", "model", "params.go")
	paramsFset := token.NewFileSet()
	paramsFile := parseKronkSource(t, paramsFset, paramsPath)
	toSampler := findKronkFunc(t, paramsFile, paramsPath, "toSampler")

	paramsSrc, err := os.ReadFile(paramsPath)
	if err != nil {
		t.Fatalf("read %s: %v", paramsPath, err)
	}

	chain := samplerChainAdds(paramsFset, root, paramsSrc, toSampler)
	if findSamplerAdd(chain, "SamplerInitPenalties") == nil {
		t.Skipf("toSampler no longer adds a penalties sampler; chain: %s", chainSummary(chain))
	}

	stateful := regexp.MustCompile(`(?i)penalt|repeat|freq|presence|dry`)

	var names []string
	for _, field := range fd.Type.Params.List {
		for _, name := range field.Names {
			names = append(names, name.Name)
			if stateful.MatchString(name.Name) {
				return
			}
		}
	}

	t.Errorf("%s: applySamplerFilters takes no penalty/DRY state; parameters are %v\n"+
		"toSampler adds Penalties (params.go:785) and DRY (params.go:791) to the\n"+
		"real chain, but speculative verification\n"+
		"(batchgen_speculative.go:498, :543, :599) computes p_target with this\n"+
		"filter instead, so accept/reject decisions and the rejection resample use\n"+
		"an unpenalized distribution. llama.cpp verifies every draft position with\n"+
		"the SAME full chain via common_sampler_sample_and_accept_n\n"+
		"(tools/server/server-context.cpp:3862, common/sampling.cpp:646-674).",
		srcPos(fset, root, fd.Pos()), names)
}

// =============================================================================
// Source-analysis helpers for the sampler chain.

// samplerAdd is one llama.SamplerInit* call inside a chain-construction
// function, in source order, together with the conditions guarding it.
type samplerAdd struct {
	name  string
	pos   string
	order int
	conds []string
}

// samplerChainAdds returns every llama.SamplerInit* call in fd in source order.
// src is the raw file content, used to render guard conditions verbatim.
func samplerChainAdds(fset *token.FileSet, root string, src []byte, fd *ast.FuncDecl) []samplerAdd {
	var (
		out   []samplerAdd
		stack []ast.Node
	)

	ast.Inspect(fd, func(n ast.Node) bool {
		if n == nil {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}

			return true
		}

		stack = append(stack, n)

		ce, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		name := samplerInitName(ce)
		if name == "" {
			return true
		}

		var conds []string
		for _, node := range stack {
			ifStmt, ok := node.(*ast.IfStmt)
			if !ok || ifStmt.Cond == nil {
				continue
			}
			conds = append(conds, nodeSource(fset, src, ifStmt.Cond))
		}

		out = append(out, samplerAdd{
			name:  name,
			pos:   srcPos(fset, root, ce.Pos()),
			order: len(out) + 1,
			conds: conds,
		})

		return true
	})

	return out
}

// samplerInitName returns the llama.SamplerInit* selector name for ce, or "".
func samplerInitName(ce *ast.CallExpr) string {
	sel, ok := ce.Fun.(*ast.SelectorExpr)
	if !ok {
		return ""
	}

	pkg, ok := sel.X.(*ast.Ident)
	if !ok || pkg.Name != "llama" {
		return ""
	}

	if !strings.HasPrefix(sel.Sel.Name, "SamplerInit") {
		return ""
	}

	return sel.Sel.Name
}

// findSamplerAdd returns the first add with the given llama.SamplerInit* name.
func findSamplerAdd(adds []samplerAdd, name string) *samplerAdd {
	for i := range adds {
		if adds[i].name == name {
			return &adds[i]
		}
	}

	return nil
}

// chainSummary renders the constructed chain as "1:TopK -> 2:TopP -> ...".
func chainSummary(adds []samplerAdd) string {
	parts := make([]string, 0, len(adds))
	for _, a := range adds {
		parts = append(parts, fmt.Sprintf("%d:%s", a.order, strings.TrimPrefix(a.name, "SamplerInit")))
	}

	return strings.Join(parts, " -> ")
}

// nodeSource returns the verbatim source text of n.
func nodeSource(fset *token.FileSet, src []byte, n ast.Node) string {
	start := fset.Position(n.Pos()).Offset
	end := fset.Position(n.End()).Offset

	if start < 0 || end > len(src) || start >= end {
		return ""
	}

	return string(src[start:end])
}
