package model

import (
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Tool-calling reproduction of the reported symptom "starts a task then drops
// it mid-way".
//
// The earlier differential run (findings2.md §13) came back INCONCLUSIVE
// because every probe was a plain conversational turn. The defects that best
// explain the report (findings2.md §8a, §8c, §12a) can only fire on a
// TOOL-CALLING turn: §12a needs a <tool_call> block open when generation
// ends, and §8a/§8c need an assistant history turn that the Qwen3.6 template
// classifies as "in-turn" (loop.index0 > ns.last_query_index), which only
// happens once a tool result follows an assistant turn.
//
// This file therefore pins the tool-calling half of the story:
//
//	TestToolCallCutOffAtMaxTokensIsInvisibleToTheCaller
//	    §12a's caller-visible consequence, plus the reason it is SILENT: Kronk
//	    has no "length" finish reason at all.
//	TestQwen36MultiStepToolLoopPromptDamage
//	    the exact prompt Kronk sends for a realistic two-step tool loop,
//	    against the template's own output.
//
// The live-model counterpart is TestToolCallDifferential in
// sdk/kronk/tests/mtp/differential_test.go (build tag kronkdiff,
// KRONK_MTP_DIFF=1), which drives the same shapes through kronk.Chat and
// through upstream llama-server.
// =============================================================================

// -----------------------------------------------------------------------------
// Local source-scanning helpers.
//
// These are deliberately private to this file rather than shared. The audit's
// shared helpers (kronkRepoRoot, parseKronkSource, posOf) live in files that
// are untracked scratch and were observed to disappear mid-session, taking the
// whole package build with them; this file must keep compiling on its own.

// toolReproRepoRoot walks up from this test file to the module root.
func toolReproRepoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("resolving the working directory: %v", err)
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod above %s", dir)
		}

		dir = parent
	}
}

// toolReproParse parses one production source file.
func toolReproParse(t *testing.T, fset *token.FileSet, path string) *ast.File {
	t.Helper()

	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	return f
}

// toolReproPos renders a node position as file:line.
func toolReproPos(fset *token.FileSet, n ast.Node) string {
	p := fset.Position(n.Pos())

	return fmt.Sprintf("%s:%d", filepath.Base(p.Filename), p.Line)
}

// -----------------------------------------------------------------------------
// Finding §12a: a tool call cut off at MaxTokens is discarded, and the caller
// is told the generation stopped normally.

// TestToolCallCutOffAtMaxTokensIsInvisibleToTheCaller pins what an API client
// receives when generation ends while the parser still holds a partial tool
// call.
//
// FINDING (findings2.md §12a, plus the missing "length" finish reason)
// The Qwen state machine buffers every byte between "<tool_call>" and
// "</tool_call>" into an internal builder and returns Result{} for each of
// them (sdk/kronk/parsers/qwen/state_machine.go:88-89; the split-tag
// lookahead at :47-68 does the same for a lone "<"). The StateMachine
// contract exposes only Classify and Reset (sdk/kronk/model/parser.go:73-80),
// so there is no way to ask for the held-back bytes. finishSlot flushes
// s.utf8Buf only (sdk/kronk/model/batchgen_finish.go:261-278) and its
// deferred s.reset() calls stateMachine.Reset()
// (sdk/kronk/model/batchgen_slot.go:379-381), which discards the builder.
//
// A second, independent route reaches the same empty response, and it is the
// one the live run below actually took: the MaxTokens cut at
// sdk/kronk/model/batchgen_tokens.go:205-210 lands on the single Result that
// carries the WHOLE tool call, and is evaluated before the accumulator write at
// :213-222. Either way finishSlot ends up with an empty s.finalTooling and
// s.toolFlag == 0, so no ResponseToolCall is ever built — see
// TestMaxTokensIsCheckedAroundBufferedToolCallTokens in this file for the exact
// ordering, and TestToolCallHeldByTheParserAtEOGIsUnrecoverable for the
// end-of-generation route.
//
// LIVE EVIDENCE (mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL, n_ctx 16384, Metal, b10211)
// TestToolCallDifferential's truncation sweep, one get_weather request per
// max_tokens value, enable_thinking=false:
//
//	max_tokens 4,8,12,16,20,24,32 -> finish_reason "stop",  content "",
//	                                 tool_calls [], no error, usage
//	                                 prompt=359 completion=39 reasoning=0
//	max_tokens 48,64             -> finish_reason "tool_calls", get_weather
//
// Seven of nine requests returned NOTHING to the caller, and usage reported 39
// completion tokens on every one of them — including the max_tokens=4 case, so
// usage does not reveal the truncation either.
//
// What makes it silent rather than merely lossy is the finish reason. Kronk
// defines exactly three (sdk/kronk/model/models.go:36-38) and
// chatResponseFinal picks between two of them
// (sdk/kronk/model/models.go:926-929):
//
//	finishReason := FinishReasonStop
//	if len(respToolCalls) > 0 {
//	    finishReason = FinishReasonTool
//	}
//
// There is no "length". A turn truncated at max_tokens is reported to the
// caller as a completed one.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/models.go:36-38    the three finish reasons
//   - sdk/kronk/model/models.go:926-929  stop unless tool calls
//   - sdk/kronk/model/batchgen_finish.go:282  tool branch over finalTooling
//
// LLAMA.CPP REFERENCE (correct behaviour)
// .extras/llama.cpp/tools/server/server-task.cpp:443 and :492 —
// to_json_oaicompat_chat and its streaming twin both START from
// finish_reason = "length" and downgrade to "stop"/"tool_calls" only for
// STOP_TYPE_WORD or STOP_TYPE_EOS. And the text itself cannot be lost:
// slot.generated_text is authoritative and send_final_response ships whatever
// remains (.extras/llama.cpp/tools/server/server-context.cpp:1868-1898).
//
// FAILURE SCENARIO (this is the user's report)
// An agent asks the model to do something, the model decides to call a tool,
// and the reply reaches max_tokens inside the <tool_call> block. Kronk
// returns HTTP 200 with content "", no tool_calls, no error, usage counting
// every generated token, and finish_reason "stop". The agent loop sees a
// well-formed empty answer, records it as the assistant's turn, and moves on:
// the task was started and then dropped, with nothing anywhere to indicate
// why.
//
// FIX: add a drain to the StateMachine contract and call it from finishSlot
// before s.reset(), and report a distinct finish reason when the MaxTokens
// budget is what ended the turn.
func TestToolCallCutOffAtMaxTokensIsInvisibleToTheCaller(t *testing.T) {
	// (a) The caller-visible response for a slot whose tool-call buffer was
	//     discarded. finishSlot's inputs after the MaxTokens cut are: empty
	//     finalContent, empty finalReasoning (thinking already closed), no
	//     respToolCalls (finalTooling was empty), and a Usage that shows the
	//     budget was consumed. chatResponseFinal is production code
	//     (sdk/kronk/model/models.go:910) and is what both transports send.
	const maxTokens = 24

	resp := chatResponseFinal(
		"chatcmpl-repro", ObjectChatText, "mtp-Qwen3.6-35B-A3B", 0, "",
		"",  // finalContent  — the tool-call bytes never reached it
		"",  // finalReasoning
		nil, // respToolCalls — finalTooling was empty at batchgen_finish.go:282
		nil,
		Usage{
			PromptTokens:     128,
			CompletionTokens: maxTokens,
			OutputTokens:     maxTokens,
			TotalTokens:      128 + maxTokens,
		},
	)

	if len(resp.Choices) != 1 {
		t.Fatalf("chatResponseFinal returned %d choices, want 1", len(resp.Choices))
	}

	choice := resp.Choices[0]

	if choice.Message == nil {
		t.Fatalf("chatResponseFinal returned a nil Message")
	}

	// The generation used its whole budget, so the caller MUST be able to
	// tell this turn was cut short. Anything but a truncation-specific
	// finish reason makes the dropped tool call indistinguishable from a
	// deliberate empty answer.
	truncated := resp.Usage != nil && resp.Usage.OutputTokens >= maxTokens
	silent := strings.TrimSpace(choice.Message.Content) == "" &&
		strings.TrimSpace(choice.Message.Reasoning) == "" &&
		len(choice.Message.ToolCalls) == 0

	if truncated && silent && choice.FinishReason() == FinishReasonStop {
		t.Errorf("a turn truncated at max_tokens with its tool call discarded is reported as a clean stop\n"+
			"got : finish_reason=%q content=%q reasoning=%q tool_calls=%d output_tokens=%d (max_tokens=%d)\n"+
			"want: a finish reason that distinguishes truncation from completion, and the\n"+
			"      partial tool call delivered rather than dropped.\n\n"+
			"chatResponseFinal (sdk/kronk/model/models.go:926-929) only ever chooses between\n"+
			"FinishReasonStop and FinishReasonTool; sdk/kronk/model/models.go:36-38 defines no\n"+
			"length/truncation reason at all. Combined with findings2.md §12a — the parser's\n"+
			"tool-call buffer (sdk/kronk/parsers/qwen/state_machine.go:88-89) is discarded by\n"+
			"stateMachine.Reset() (sdk/kronk/model/batchgen_slot.go:379-381) because nothing\n"+
			"drains it (sdk/kronk/model/parser.go:73-80,\n"+
			"sdk/kronk/model/batchgen_finish.go:261-278) — an agent that hits its token budget\n"+
			"mid-<tool_call> receives a well-formed EMPTY assistant turn and no error.\n"+
			"llama.cpp starts from finish_reason \"length\" and only downgrades it for a real\n"+
			"stop token or stop word (.extras/llama.cpp/tools/server/server-task.cpp:443,:492),\n"+
			"and it never loses the text (server-context.cpp:1868-1898).",
			choice.FinishReason(), choice.Message.Content, choice.Message.Reasoning,
			len(choice.Message.ToolCalls), resp.Usage.OutputTokens, maxTokens)
	}

	// (b) Source-level confirmation that the missing reason is missing on
	//     purpose-free grounds: enumerate the FinishReason constants so the
	//     failure above cannot be dismissed as a test artefact.
	fset := token.NewFileSet()
	f := toolReproParse(t, fset, filepath.Join(toolReproRepoRoot(t), "sdk", "kronk", "model", "models.go"))

	var reasons []string
	ast.Inspect(f, func(n ast.Node) bool {
		vs, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}

		for _, name := range vs.Names {
			if strings.HasPrefix(name.Name, "FinishReason") {
				reasons = append(reasons, name.Name)
			}
		}

		return true
	})

	t.Logf("finish reasons defined in sdk/kronk/model/models.go: %s", strings.Join(reasons, ", "))

	for _, r := range reasons {
		if strings.Contains(r, "Length") || strings.Contains(r, "Truncat") {
			return
		}
	}

	t.Errorf("no truncation finish reason exists: models.go defines only %s\n"+
		"llama.cpp reports finish_reason \"length\" whenever the token budget, not a stop\n"+
		"token, ended the turn (.extras/llama.cpp/tools/server/server-task.cpp:410,:443,:492).\n"+
		"Without it, findings2.md §12a is undetectable by any client: a discarded tool call\n"+
		"and a deliberate empty reply are byte-identical responses.",
		strings.Join(reasons, ", "))
}

// TestMaxTokensIsCheckedAroundBufferedToolCallTokens pins the statement order
// in handleSampledToken that destroys a COMPLETE tool call, measured live.
//
// FINDING
// This is the mechanism behind the live reproduction recorded by
// TestToolCallDifferential (sdk/kronk/tests/mtp/differential_test.go). Driving
// mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL through kronk.Chat with one tool and
// enable_thinking=false, sweeping max_tokens:
//
//	max_tokens  4  8 12 16 20 24 32 36 38 39 -> finish_reason "stop",
//	                                            content "", tool_calls 0,
//	                                            error nil, output_tokens 39
//	max_tokens 40 41 44 48 64 200            -> finish_reason "tool_calls",
//	                                            get_weather delivered,
//	                                            output_tokens 39
//
// Two separate defects produce that table, and both are visible as an ORDERING
// problem in handleSampledToken:
//
//  1. MaxTokens is not enforced at all while the parser buffers. Every token
//     of a <tool_call> body classifies as ChannelNone (the Qwen state machine
//     accumulates them and returns Result{}, parsers/qwen/state_machine.go:88-89),
//     and sdk/kronk/model/batchgen_tokens.go:200-203 returns for ChannelNone
//     BEFORE the budget comparison at :207. That is why a cap of 4 still
//     generated 39 tokens — a ~10x overrun of an explicit limit.
//
//  2. The whole payload is lost in one step. The state machine releases the
//     entire buffered call as a single Result at the closing tag
//     (parsers/qwen/state_machine.go:80-89). On exactly that token the budget
//     check at :207-210 finally runs, sees outputTokens (39) >= MaxTokens, and
//     calls finishSlot BEFORE the store at :213-222 — so s.finalTooling stays
//     empty and finishSlot's tool branch (batchgen_finish.go:282) has nothing
//     to parse. The failure boundary is therefore exactly the token count:
//     cap <= 39 loses the call, cap >= 40 keeps it, and the loss is total
//     rather than one token as in TestMaxTokensKeepsTheLimitReachingTokensText.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_tokens.go:200-203  ChannelNone early return
//   - sdk/kronk/model/batchgen_tokens.go:207-210  budget check
//   - sdk/kronk/model/batchgen_tokens.go:213-222  content store
//
// LLAMA.CPP REFERENCE (correct behaviour)
// .extras/llama.cpp/tools/server/server-context.cpp:1895-1899 adds the token
// to slot.generated_text and sends the partial response FIRST; the budget test
// that sets STOP_TYPE_LIMIT is at :1913-1918, after it. There is no channel
// classification that can skip the budget test, and because generated_text is
// the authority, send_final_response (:1868-1898) still ships the text with
// finish_reason "length" (server-task.cpp:443).
//
// FAILURE SCENARIO
// An agent framework that sets any max_tokens at or below the length of the
// model's tool call gets HTTP 200, an empty assistant message, no tool call
// and no error. It records the empty turn and continues: the announced action
// never happens. Meanwhile a runaway tool call ignores the cap completely.
//
// FIX: run the budget check for every token (move it above the ChannelNone
// return) and store/stream the content before finishing, as llama.cpp does.
func TestMaxTokensIsCheckedAroundBufferedToolCallTokens(t *testing.T) {
	path := filepath.Join(toolReproRepoRoot(t), "sdk", "kronk", "model", "batchgen_tokens.go")

	fset := token.NewFileSet()
	f := toolReproParse(t, fset, path)

	var fn *ast.FuncDecl
	for _, decl := range f.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if ok && fd.Name.Name == "handleSampledToken" && fd.Body != nil {
			fn = fd
			break
		}
	}

	if fn == nil {
		t.Fatalf("handleSampledToken not found in %s", path)
	}

	// Positions of the four statements whose order decides whether a
	// completed tool call survives its own token budget. handleSampledToken
	// contains TWO MaxTokens checks: one in the partial-UTF-8 branch
	// (batchgen_tokens.go:148), reached before the parser is consulted, and
	// the main-path one (:207). Only the main path matters here, so the
	// classify call is the anchor that separates them.
	var classifyPos, nonePos, storePos token.Pos
	var budgetPositions []token.Pos

	ast.Inspect(fn.Body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.IfStmt:
			cond, ok := node.Cond.(*ast.BinaryExpr)
			if !ok {
				return true
			}

			// `result.Channel == ChannelNone`
			if id, ok := cond.Y.(*ast.Ident); ok && id.Name == "ChannelNone" && nonePos == token.NoPos {
				nonePos = node.Pos()
			}

			// `outputTokens >= s.job.params.MaxTokens`
			if sel, ok := cond.Y.(*ast.SelectorExpr); ok && sel.Sel.Name == "MaxTokens" {
				budgetPositions = append(budgetPositions, node.Pos())
			}

		case *ast.CallExpr:
			sel, ok := node.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}

			// `s.stateMachine.Classify(content)`
			if sel.Sel.Name == "Classify" && classifyPos == token.NoPos {
				classifyPos = node.Pos()
			}

			// `s.finalTooling.WriteString(result.Content)`
			if sel.Sel.Name == "WriteString" {
				if inner, ok := sel.X.(*ast.SelectorExpr); ok &&
					inner.Sel.Name == "finalTooling" && storePos == token.NoPos {
					storePos = node.Pos()
				}
			}
		}

		return true
	})

	if classifyPos == token.NoPos || nonePos == token.NoPos || storePos == token.NoPos ||
		len(budgetPositions) == 0 {
		t.Fatalf("could not locate the statements in handleSampledToken "+
			"(Classify %v, ChannelNone return %v, finalTooling store %v, %d MaxTokens checks) — "+
			"the function was restructured; re-derive this pin against %s",
			classifyPos.IsValid(), nonePos.IsValid(), storePos.IsValid(),
			len(budgetPositions), path)
	}

	// mainBudgetPos is the budget check on the classified-token path.
	var mainBudgetPos token.Pos
	var guardsBufferedTokens bool
	for _, p := range budgetPositions {
		if p > classifyPos && p < nonePos {
			// A budget check between Classify and the ChannelNone return
			// WOULD cover a buffered tool-call token. This is the fix.
			guardsBufferedTokens = true
		}

		if p > nonePos && (mainBudgetPos == token.NoPos || p < mainBudgetPos) {
			mainBudgetPos = p
		}
	}

	pos := func(p token.Pos) string { return toolReproPos(fset, &ast.Ident{NamePos: p}) }

	var budgetLines []string
	for _, p := range budgetPositions {
		budgetLines = append(budgetLines, pos(p))
	}

	t.Logf("handleSampledToken order: Classify %s, ChannelNone return %s, "+
		"finalTooling store %s, MaxTokens checks %v",
		pos(classifyPos), pos(nonePos), pos(storePos), budgetLines)

	if !guardsBufferedTokens {
		t.Errorf("no MaxTokens budget check sits between the parser call (%s) and the "+
			"ChannelNone early return (%s), so the budget is never evaluated for a token "+
			"the parser is buffering. The checks in this function are at %v.\n"+
			"Every token inside a <tool_call> block classifies as ChannelNone "+
			"(sdk/kronk/parsers/qwen/state_machine.go:88-89) and returns at "+
			"sdk/kronk/model/batchgen_tokens.go:200-203, so an explicit max_tokens is "+
			"ignored for the whole call. Measured live against "+
			"mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL through kronk.Chat: max_tokens=4 still produced "+
			"39 output tokens.\n"+
			"llama.cpp evaluates the budget for every accepted token, with no channel "+
			"classification in front of it "+
			"(.extras/llama.cpp/tools/server/server-context.cpp:1913-1918).",
			pos(classifyPos), pos(nonePos), budgetLines)
	}

	if mainBudgetPos == token.NoPos {
		t.Fatalf("no MaxTokens check after the ChannelNone return in handleSampledToken; "+
			"re-derive this pin against %s", path)
	}

	if storePos > mainBudgetPos {
		t.Errorf("the tool-call content store (%s) comes AFTER the MaxTokens budget check (%s), "+
			"so the token that trips the budget is discarded — and for a tool call that token "+
			"carries the ENTIRE payload\n"+
			"The Qwen state machine releases the whole buffered call as one Result at the "+
			"closing tag (sdk/kronk/parsers/qwen/state_machine.go:80-89), so finishSlot's tool "+
			"branch (sdk/kronk/model/batchgen_finish.go:282) runs over an empty s.finalTooling "+
			"and the caller receives an empty message with finish_reason \"stop\" and no error. "+
			"Measured live: max_tokens<=39 lost a complete get_weather call, max_tokens>=40 "+
			"delivered it, on identical requests generating 39 tokens.\n"+
			"llama.cpp appends to slot.generated_text and streams the token first "+
			"(.extras/llama.cpp/tools/server/server-context.cpp:1895-1899), then applies the "+
			"budget (:1913-1918), and reports finish_reason \"length\" "+
			"(tools/server/server-task.cpp:443).",
			pos(storePos), pos(mainBudgetPos))
	}
}

// -----------------------------------------------------------------------------
// Findings §8a + §8c on a realistic multi-step tool loop.

// qwen36ToolLoopPrompt renders msgs through the production sequence a
// tool-calling chat request actually takes:
//
//	Model.prepareContext        (chat.go:253)  -> normalizeHistoryReasoning
//	Model.prepareCacheAndPrompt (chat.go:281)  -> createPrompt
//	Model.applyRequestJinjaTemplate (prompts.go:23) -> applyJinjaTemplate
//
// The two stages that damage the prompt are normalizeHistoryReasoning
// (reasoning.go:46) and the post-render strip inside applyJinjaTemplate
// (prompts.go:177-181); both are reached here. Passing imc=false disables
// both (reasoning.go:37) and yields the template's own output, which is the
// reference this test compares against.
//
// It reuses newQwen36PromptModel from prompt_tokenize_special_test.go, which
// carries the VERBATIM tokenizer.chat_template of
// mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf.
func qwen36ToolLoopPrompt(t *testing.T, imc bool, msgs []D) string {
	t.Helper()

	m := newQwen36PromptModel(imc)

	d := D{
		"messages": msgs,
		// A tool-calling request always carries the tool schema; it is
		// rendered into a leading system block (qwen3.6.jinja:58-70) that is
		// irrelevant here, so this test omits it and asserts on the
		// conversation body only.
		"enable_thinking": true,
		"bos_token":       "<|endoftext|>",
		"eos_token":       "<|im_end|>",
	}

	got, err := m.applyJinjaTemplate(context.Background(), m.normalizeHistoryReasoning(d))
	if err != nil {
		t.Fatalf("rendering the tool-loop prompt: %v", err)
	}

	return got
}

// TestQwen36MultiStepToolLoopPromptDamage pins the exact prompt Kronk sends
// on the second step of a two-tool-call agentic loop.
//
// FINDING (findings2.md §8a and §8c, composed on one realistic conversation)
// The Qwen3.6 template treats every assistant message after the last real
// user query as "in-turn" and replays its reasoning verbatim
// (.extras/templates/qwen3.6.jinja:103-104, byte-identical to the GGUF's
// embedded tokenizer.chat_template):
//
//	{%- if (preserve_thinking ...) or (loop.index0 > ns.last_query_index) %}
//	    {{- '<|im_start|>' + message.role + '\n<think>\n' + reasoning_content + '\n</think>\n\n' + content }}
//
// ns.last_query_index is the index of the last user message whose content is
// not a <tool_response> wrapper (:76-86), so in a tool loop EVERY assistant
// turn after the user's request is in-turn. Kronk damages all of them twice:
//
//  1. normalizeHistoryReasoning (sdk/kronk/model/reasoning.go:91-92) deletes
//     the reasoning and reasoning_content fields from every assistant
//     message, so the template renders an empty span — the model no longer
//     sees the reasoning that justified its own tool call (§8c).
//  2. The post-render pass (sdk/kronk/model/prompts.go:177-181 ->
//     sdk/kronk/parsers/standard/reasoning.go:35,43) then deletes the empty
//     "<think>...</think>" span but not the "\n\n" the template emits inside
//     the same literal, so the assistant header degrades from
//     "<|im_start|>assistant\n<think>\n\n</think>\n\n" to
//     "<|im_start|>assistant\n\n\n" — the two USER_DEFINED tokens 248068 and
//     248069 become ordinary newlines on every step of the loop (§8a).
//
// Both are gated only on Config.IncrementalCache() (reasoning.go:37), which
// is on by default, so no client cooperation is needed.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/reasoning.go:91-92    reasoning fields deleted
//   - sdk/kronk/model/prompts.go:177-181    post-render strip over the prompt
//
// LLAMA.CPP REFERENCE (correct behaviour)
// .extras/llama.cpp/common/chat.cpp:895-900 hands inputs.messages to the
// template unchanged — there is no reasoning-stripping pass anywhere in the
// request path — and :926-940 shows the only post-render edit is BOS/EOS
// de-duplication. Upstream's own workarounds run BEFORE the render and only
// ADD required fields (chat.cpp:3082-3085 parses string tool arguments into
// objects, :3079 fills null content); none of them remove content.
//
// FAILURE SCENARIO
// A multi-step agent: the model reasons "the queue is unbounded, I should
// read svc/queue.go", calls read_file, and gets the file back. On the next
// step Kronk shows it an assistant turn with the reasoning deleted and the
// reasoning delimiters replaced by blank lines. The model has to re-derive
// why it called the tool, from a turn whose channel structure no longer
// parses — which is exactly "it contradicts what it already said" and "it
// starts a task then drops it".
func TestQwen36MultiStepToolLoopPromptDamage(t *testing.T) {
	// A realistic two-step loop: list the files, then read one. Both
	// assistant turns carry the reasoning the model produced, as an
	// OpenAI-style client replays it.
	msgs := []D{
		{"role": "system", "content": "You audit Go repositories with tools."},
		{"role": "user", "content": "Audit the repo and report every defect."},
		{
			"role":              "assistant",
			"content":           "",
			"reasoning_content": "I do not know the file list yet, so list_files must come first.",
			"tool_calls": []D{
				{"function": D{"name": "list_files", "arguments": D{}}},
			},
		},
		{"role": "tool", "name": "list_files", "content": "svc/queue.go"},
		{
			"role":              "assistant",
			"content":           "",
			"reasoning_content": "One file. Read svc/queue.go before recording anything.",
			"tool_calls": []D{
				{"function": D{"name": "read_file", "arguments": D{"path": "svc/queue.go"}}},
			},
		},
		{"role": "tool", "name": "read_file", "content": "func NewQueue() chan Job { return make(chan Job) }"},
	}

	// What the template itself produces for these messages. Reasoning is
	// replayed on both in-turn assistant messages (qwen3.6.jinja:104) and the
	// <think>/</think> delimiters are present, as llama.cpp would send them.
	const want = "<|im_start|>system\nYou audit Go repositories with tools.<|im_end|>\n" +
		"<|im_start|>user\nAudit the repo and report every defect.<|im_end|>\n" +
		"<|im_start|>assistant\n<think>\nI do not know the file list yet, so list_files must come first.\n</think>\n\n" +
		"<tool_call>\n<function=list_files>\n</function>\n</tool_call><|im_end|>\n" +
		"<|im_start|>user\n<tool_response>\nsvc/queue.go\n</tool_response><|im_end|>\n" +
		"<|im_start|>assistant\n<think>\nOne file. Read svc/queue.go before recording anything.\n</think>\n\n" +
		"<tool_call>\n<function=read_file>\n<parameter=path>\nsvc/queue.go\n</parameter>\n</function>\n</tool_call><|im_end|>\n" +
		"<|im_start|>user\n<tool_response>\nfunc NewQueue() chan Job { return make(chan Job) }\n</tool_response><|im_end|>\n" +
		"<|im_start|>assistant\n<think>\n"

	got := qwen36ToolLoopPrompt(t, true, msgs)
	if got == want {
		return
	}

	reference := qwen36ToolLoopPrompt(t, false, msgs)

	// Quantify the damage so the failure is readable at a glance: how many
	// reasoning delimiters the template emitted versus how many survive, and
	// which reasoning text was deleted outright.
	wantOpen := strings.Count(want, "<think>")
	gotOpen := strings.Count(got, "<think>")
	wantClose := strings.Count(want, "</think>")
	gotClose := strings.Count(got, "</think>")

	var lostReasoning []string
	for _, msg := range msgs {
		r, ok := msg["reasoning_content"].(string)
		if ok && r != "" && !strings.Contains(got, r) {
			lostReasoning = append(lostReasoning, r)
		}
	}

	t.Errorf("the prompt Kronk sends for a two-step tool loop is not the prompt the template produced\n"+
		"got  (Kronk, IncrementalCache on):\n%q\n\n"+
		"want (template / llama.cpp):\n%q\n\n"+
		"same messages with IncrementalCache off (i.e. both passes disabled):\n%q\n\n"+
		"<think> openers: %d -> %d, </think> closers: %d -> %d\n"+
		"reasoning deleted from history entirely: %q\n\n"+
		"Two production stages do this. normalizeHistoryReasoning\n"+
		"(sdk/kronk/model/reasoning.go:91-92) deletes reasoning_content from every\n"+
		"assistant message, although qwen3.6.jinja:103-104 replays it for in-turn turns —\n"+
		"which every assistant turn of a tool loop is (:76-86). The post-render strip\n"+
		"(sdk/kronk/model/prompts.go:177-181 -> sdk/kronk/parsers/standard/reasoning.go:35,43)\n"+
		"then removes the empty <think></think> span but leaves the '\\n\\n' from the same\n"+
		"template literal, so each assistant header becomes \"<|im_start|>assistant\\n\\n\\n\"\n"+
		"and tokens 248068/248069 degrade into newlines.\n"+
		"llama.cpp passes messages to the template verbatim\n"+
		"(.extras/llama.cpp/common/chat.cpp:895-900) and edits the render only to\n"+
		"de-duplicate BOS/EOS (:926-940).\n"+
		"Fix: stop deleting reasoning the template asks for, and drop the post-render pass.",
		got, want, reference, wantOpen, gotOpen, wantClose, gotClose, lostReasoning)
}

// -----------------------------------------------------------------------------
// Finding §12a, the OTHER route into it: end-of-generation while the parser is
// still buffering.
//
// TestMaxTokensIsCheckedAroundBufferedToolCallTokens above covers the route the
// live sweep took, in which the model DID close its tool call and the budget
// check threw the flush away. This section covers the route the audit
// originally described: generation ends with the closing tag never emitted, so
// the bytes are still inside the parser. max_tokens cannot produce that state
// (the budget is not evaluated for ChannelNone tokens at all), but the EOG
// check at sdk/kronk/model/batchgen_tokens.go:66-69 can, and it is not
// reachable from a test that only varies max_tokens.

// qwenToolBufferMirror is a VERBATIM MIRROR of the tool-call buffering half of
// the Qwen state machine:
//
//	sdk/kronk/parsers/qwen/state_machine.go:134-138  opener -> inToolCall, ChannelNone
//	sdk/kronk/parsers/qwen/state_machine.go:90-106   body   -> buffered, ChannelNone
//	sdk/kronk/parsers/qwen/state_machine.go:80-88    closer -> the WHOLE buffer,
//	                                                          ChannelTool, in one Result
//
// The real state machine cannot be used from package model: sdk/kronk/parsers/qwen
// imports this package, so importing it back is an import cycle. Only the three
// transitions above are mirrored — the <think> handling, the split-<function=
// lookahead and the implicit </function> close are irrelevant here and are
// deliberately omitted.
//
// held() is what makes this test possible and is exactly what production cannot
// do: the model.StateMachine contract (sdk/kronk/model/parser.go:73-80) has no
// such method, which is the defect.
//
// MAINTAINER: when parsers/qwen/state_machine.go changes its tool-call
// buffering, update this mirror in the same commit.
type qwenToolBufferMirror struct {
	buf        strings.Builder
	inToolCall bool
}

func (sm *qwenToolBufferMirror) classify(content string) Result {
	if sm.inToolCall {
		switch content {
		case "<tool_call>", "<|tool_call>":
			return Result{}

		case "</tool_call>", "<tool_call|>":
			toolContent := strings.Trim(sm.buf.String(), "\n")
			if toolContent != "" {
				toolContent = fmt.Sprintf("%s\n", toolContent)
			}
			sm.buf.Reset()
			sm.inToolCall = false

			return Result{Channel: ChannelTool, Content: toolContent}

		default:
			sm.buf.WriteString(content)

			return Result{}
		}
	}

	switch content {
	case "<tool_call>", "<|tool_call>":
		sm.inToolCall = true
		sm.buf.Reset()

		return Result{}

	default:
		return Result{Channel: ChannelAnswer, Content: content}
	}
}

// held reports the bytes the mirror is withholding. Production has no
// equivalent, which is the whole point.
func (sm *qwenToolBufferMirror) held() string { return sm.buf.String() }

// toolSlotMirror is a VERBATIM MIRROR of the accumulator half of a slot, as
// driven by handleSampledToken and then read by finishSlot:
//
//	sdk/kronk/model/batchgen_tokens.go:166-181  the flag switch
//	sdk/kronk/model/batchgen_tokens.go:188-193  token counting
//	sdk/kronk/model/batchgen_tokens.go:200-203  ChannelNone early return
//	sdk/kronk/model/batchgen_tokens.go:213-222  the accumulator store
//
// The MaxTokens comparison at :205-210 is deliberately NOT mirrored: this test
// is about the EOG route, where the budget never binds.
//
// MAINTAINER: keep in step with handleSampledToken.
type toolSlotMirror struct {
	reasonFlag     int
	completionFlag int
	toolFlag       int

	reasonTokens     int
	completionTokens int

	finalContent   strings.Builder
	finalReasoning strings.Builder
	finalTooling   strings.Builder
}

func (s *toolSlotMirror) feed(r Result) {
	switch r.Channel {
	case ChannelReasoning:
		s.reasonFlag++
		s.completionFlag = 0
		s.toolFlag = 0

	case ChannelAnswer:
		s.completionFlag++
		s.reasonFlag = 0
		s.toolFlag = 0

	case ChannelTool:
		s.toolFlag++
		s.reasonFlag = 0
		s.completionFlag = 0
	}

	switch {
	case s.reasonFlag > 0:
		s.reasonTokens++
	default:
		s.completionTokens++
	}

	if r.Channel == ChannelNone {
		return
	}

	switch {
	case s.reasonFlag > 0:
		s.finalReasoning.WriteString(r.Content)

	case s.toolFlag > 0:
		s.finalTooling.WriteString(r.Content)

	default:
		s.finalContent.WriteString(r.Content)
	}
}

// TestToolCallHeldByTheParserAtEOGIsUnrecoverable pins findings2 §12a on the
// end-of-generation route: an unterminated <tool_call> is destroyed, and
// finishSlot cannot even tell it happened.
//
// FINDING (findings2.md §12a)
// The Qwen state machine buffers the tool-call body and returns Result{} for
// every token of it (sdk/kronk/parsers/qwen/state_machine.go:90-106), releasing
// it only at the closing tag (:80-88). If the model emits an EOG token first —
// <|im_end|> straight after the JSON, a degenerate turn, a truncated
// <function=…> — handleSampledToken finishes the slot at
// sdk/kronk/model/batchgen_tokens.go:66-69 without ever consulting the parser.
// The StateMachine contract has no drain (sdk/kronk/model/parser.go:73-80);
// finishSlot flushes s.utf8Buf and nothing else
// (sdk/kronk/model/batchgen_finish.go:261-278); the deferred s.reset() then
// calls stateMachine.Reset() (sdk/kronk/model/batchgen_slot.go:379-381) and the
// builder is gone.
//
// Two consequences this test measures, both invisible from production code:
//
//  1. every accumulator is empty, so sendFinalResponse ships content "",
//     reasoning "" and no tool calls;
//  2. s.toolFlag is still 0 — it is only incremented for ChannelTool
//     (sdk/kronk/model/batchgen_tokens.go:177-180), and the buffered tokens are
//     ChannelNone — so finishSlot does not even ENTER the tool branch at
//     sdk/kronk/model/batchgen_finish.go:282. There is no log line, no
//     parse-error, no diagnostic of any kind.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_tokens.go:66-69     EOG -> finishSlot, no drain
//   - sdk/kronk/model/batchgen_finish.go:261-278   only utf8Buf is flushed
//   - sdk/kronk/model/batchgen_slot.go:379-381     Reset() discards the buffer
//
// The contract-level statement of the same defect (StateMachine exposes no
// drain, finishSlot never touches s.stateMachine) is
// TestGenerationEndDrainsParserHeldBackContent in
// sdk/kronk/model/nonspeculative_decode_test.go. This test is its behavioural
// counterpart: it measures what is lost, not just that no API exists to
// recover it.
//
// LLAMA.CPP REFERENCE (correct behaviour)
// .extras/llama.cpp/tools/server/server-context.cpp:1868-1898 —
// slot.generated_text is the authority. Text withheld from the stream while a
// partial stop string is pending is never deleted from it, and
// send_final_response ships whatever remains, so an unterminated tag cannot
// swallow the model's output.
//
// FAILURE SCENARIO
// A reasoning model in an agentic loop opens <tool_call>, writes the JSON, and
// emits <|im_end|> without the closing tag. The caller receives a well-formed
// empty assistant turn with finish_reason "stop" and no error, records it as
// the model's answer, and moves on — the announced action never happens. This
// is the reported "starts a task then drops it mid-way".
//
// FIX: add a drain to the StateMachine contract and call it from finishSlot
// before s.reset(), or keep the raw generated text on the slot as the
// authority the way llama.cpp does.
func TestToolCallHeldByTheParserAtEOGIsUnrecoverable(t *testing.T) {
	// One turn as the tokenizer fragments it: the opener, the JSON body, and
	// then EOG — no "</tool_call>".
	tokens := []string{
		"<tool_call>",
		"\n", "{\"name\":", " \"get_weather\",", " \"arguments\":",
		" {\"location\":", " \"London,", " United", " Kingdom\",",
		" \"units\":", " \"celsius\"}}", "\n",
	}

	sm := &qwenToolBufferMirror{}
	s := &toolSlotMirror{}

	for _, tok := range tokens {
		s.feed(sm.classify(tok))
	}

	// llama.VocabIsEOG(...) is true for the next token, so
	// handleSampledToken returns at batchgen_tokens.go:66-69 and finishSlot
	// runs. finishSlot flushes s.utf8Buf (empty here: every piece was a
	// complete codepoint) and nothing else.
	produced := strings.Join(tokens[1:], "")
	delivered := s.finalContent.String() + s.finalReasoning.String() + s.finalTooling.String()

	if strings.Contains(delivered, "get_weather") {
		return
	}

	t.Errorf("an unterminated tool call is destroyed at end-of-generation: the model produced %d bytes, the caller receives %d\n"+
		"produced (by the model)      : %q\n"+
		"still inside the parser      : %q\n"+
		"delivered to the caller      : %q\n"+
		"slot state finishSlot sees   : toolFlag=%d completionTokens=%d reasonTokens=%d "+
		"finalContent=%d finalReasoning=%d finalTooling=%d bytes\n\n"+
		"The Qwen state machine buffers the body and returns Result{} for every token of it\n"+
		"(sdk/kronk/parsers/qwen/state_machine.go:90-106), releasing it only at the closing tag\n"+
		"(:80-88). On EOG, handleSampledToken calls finishSlot at\n"+
		"sdk/kronk/model/batchgen_tokens.go:66-69 without consulting the parser; the\n"+
		"StateMachine contract has no drain (sdk/kronk/model/parser.go:73-80), finishSlot\n"+
		"flushes only s.utf8Buf (sdk/kronk/model/batchgen_finish.go:261-278), and the deferred\n"+
		"s.reset() calls stateMachine.Reset() (sdk/kronk/model/batchgen_slot.go:379-381).\n"+
		"Note toolFlag=0: ChannelNone never increments it "+
		"(sdk/kronk/model/batchgen_tokens.go:177-180), so finishSlot does not even enter the\n"+
		"tool branch at sdk/kronk/model/batchgen_finish.go:282 — nothing is logged and no error\n"+
		"is returned. The caller gets content \"\", no tool_calls and finish_reason \"stop\"\n"+
		"(sdk/kronk/model/models.go:926-929), i.e. a task announced and then silently dropped.\n"+
		"llama.cpp cannot lose it: slot.generated_text is authoritative and send_final_response\n"+
		"ships the remainder (.extras/llama.cpp/tools/server/server-context.cpp:1868-1898).",
		len(produced), len(delivered), produced, sm.held(), delivered,
		s.toolFlag, s.completionTokens, s.reasonTokens,
		s.finalContent.Len(), s.finalReasoning.Len(), s.finalTooling.Len())
}
