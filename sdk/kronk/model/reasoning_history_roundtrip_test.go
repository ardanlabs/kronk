package model

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// =============================================================================
// Reasoning history round-trip: Qwen3.6-35B-A3B (MTP)
//
// GROUND TRUTH used by every test in this file is the chat template embedded in
// the model itself:
//
//	~/.kronk/models/unsloth/Qwen3.6-35B-A3B-MTP-GGUF/mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf
//	  general.architecture      = "qwen35moe"   -> parsers/qwen claims it (qwen.go:35)
//	  tokenizer.chat_template   = qwen36ChatTemplate below (8057 bytes, verbatim)
//	  "<think>"  = token 248068  (single special token)
//	  "</think>" = token 248069  (single special token)
//
// The template is reproduced verbatim below; the line numbers cited in the test
// doc comments are its own 1-based line numbers. The two rules that matter:
//
//	L92-107   assistant-turn rendering. L103-104 emit
//	          "<think>\n" + reasoning_content + "\n</think>\n\n" + content
//	          whenever preserve_thinking is true OR the assistant turn sits
//	          AFTER the last real user query (loop.index0 > ns.last_query_index,
//	          i.e. every turn of a tool-call loop and any trailing assistant
//	          turn). Otherwise (plain prior turns) reasoning is dropped.
//	L150-157  generation prompt: "<|im_start|>assistant\n" followed by
//	          "<think>\n\n</think>\n\n" when enable_thinking is false, else a
//	          FORCE-OPENED "<think>\n".
//
// llama.cpp is the reference implementation for the surrounding behaviour:
//
//	common/chat.cpp:895-900   the template receives inputs.messages VERBATIM.
//	                          Nothing in common/chat.cpp removes reasoning_content
//	                          from history; the template alone decides.
//	common/chat.cpp:949-971   generation_prompt = the suffix diff between the
//	                          renders with and without add_generation_prompt,
//	                          i.e. "<|im_start|>assistant\n<think>\n" here.
//	common/chat.cpp:3252-3254 that generation prompt is PREPENDED to the model
//	                          output before parsing, which is how llama.cpp
//	                          copes with a force-opened <think>. Kronk does the
//	                          equivalent at batchgen_slot_start.go:37-53.
// =============================================================================

const qwen36ChatTemplate = `{%- set image_count = namespace(value=0) %}
{%- set video_count = namespace(value=0) %}
{%- macro render_content(content, do_vision_count, is_system_content=false) %}
    {%- if content is string %}
        {{- content }}
    {%- elif content is iterable and content is not mapping %}
        {%- for item in content %}
            {%- if 'image' in item or 'image_url' in item or item.type == 'image' %}
                {%- if is_system_content %}
                    {{- raise_exception('System message cannot contain images.') }}
                {%- endif %}
                {%- if do_vision_count %}
                    {%- set image_count.value = image_count.value + 1 %}
                {%- endif %}
                {%- if add_vision_id %}
                    {{- 'Picture ' ~ image_count.value ~ ': ' }}
                {%- endif %}
                {{- '<|vision_start|><|image_pad|><|vision_end|>' }}
            {%- elif 'video' in item or item.type == 'video' %}
                {%- if is_system_content %}
                    {{- raise_exception('System message cannot contain videos.') }}
                {%- endif %}
                {%- if do_vision_count %}
                    {%- set video_count.value = video_count.value + 1 %}
                {%- endif %}
                {%- if add_vision_id %}
                    {{- 'Video ' ~ video_count.value ~ ': ' }}
                {%- endif %}
                {{- '<|vision_start|><|video_pad|><|vision_end|>' }}
            {%- elif 'text' in item %}
                {{- item.text }}
            {%- else %}
                {{- raise_exception('Unexpected item type in content.') }}
            {%- endif %}
        {%- endfor %}
    {%- elif content is none or content is undefined %}
        {{- '' }}
    {%- else %}
        {{- raise_exception('Unexpected content type.') }}
    {%- endif %}
{%- endmacro %}
{%- if not messages %}
    {{- raise_exception('No messages provided.') }}
{%- endif %}
{%- set num_sys = 0 %}
{%- set merged_system = '' %}
{%- if messages[0].role == 'system' or messages[0].role == 'developer' %}
    {%- set first = render_content(messages[0].content, false, true)|trim %}
    {%- if messages|length > 1 and (messages[1].role == 'system' or messages[1].role == 'developer') %}
        {%- set second = render_content(messages[1].content, false, true)|trim %}
        {%- set merged_system = first + '\n' + second %}
        {%- set num_sys = 2 %}
    {%- else %}
        {%- set merged_system = first %}
        {%- set num_sys = 1 %}
    {%- endif %}
{%- endif %}
{%- if tools and tools is iterable and tools is not mapping %}
    {{- '<|im_start|>system\n' }}
    {{- "# Tools\n\nYou have access to the following functions:\n\n<tools>" }}
    {%- for tool in tools %}
        {{- "\n" }}
        {{- tool | tojson }}
    {%- endfor %}
    {{- "\n</tools>" }}
    {{- '\n\nIf you choose to call a function ONLY reply in the following format with NO suffix:\n\n<tool_call>\n<function=example_function_name>\n<parameter=example_parameter_1>\nvalue_1\n</parameter>\n<parameter=example_parameter_2>\nThis is the value for the second parameter\nthat can span\nmultiple lines\n</parameter>\n</function>\n</tool_call>\n\n<IMPORTANT>\nReminder:\n- Function calls MUST follow the specified format: an inner <function=...></function> block must be nested within <tool_call></tool_call> XML tags\n- Required parameters MUST be specified\n- You may provide optional reasoning for your function call in natural language BEFORE the function call, but NOT after\n- If there is no function call available, answer the question like normal with your current knowledge and do not tell the user about function calls\n</IMPORTANT>' }}
    {%- if merged_system %}
        {{- '\n\n' + merged_system }}
    {%- endif %}
    {{- '<|im_end|>\n' }}
{%- else %}
    {%- if merged_system %}
        {{- '<|im_start|>system\n' + merged_system + '<|im_end|>\n' }}
    {%- endif %}
{%- endif %}
{%- set ns = namespace(multi_step_tool=true, last_query_index=messages|length - 1) %}
{%- for message in messages[::-1] %}
    {%- set index = (messages|length - 1) - loop.index0 %}
    {%- if ns.multi_step_tool and message.role == "user" %}
        {%- set content = render_content(message.content, false)|trim %}
        {%- if not(content.startswith('<tool_response>') and content.endswith('</tool_response>')) %}
            {%- set ns.multi_step_tool = false %}
            {%- set ns.last_query_index = index %}
        {%- endif %}
    {%- endif %}
{%- endfor %}
{%- for message in messages %}
    {%- if loop.index0 >= num_sys and message.role != "system" and message.role != "developer" %}
    {%- set content = render_content(message.content, true)|trim %}
    {%- if message.role == "user" %}
        {{- '<|im_start|>' + message.role + '\n' + content + '<|im_end|>' + '\n' }}
    {%- elif message.role == "assistant" %}
        {%- set reasoning_content = '' %}
        {%- if message.reasoning_content is string %}
            {%- set reasoning_content = message.reasoning_content %}
        {%- else %}
            {%- if '</think>' in content %}
                {%- set reasoning_content = content.split('</think>')[0].rstrip('\n').split('<think>')[-1].lstrip('\n') %}
                {%- set content = content.split('</think>')[-1].lstrip('\n') %}
            {%- endif %}
        {%- endif %}
        {%- set reasoning_content = reasoning_content|trim %}
        {%- if (preserve_thinking is defined and preserve_thinking is true) or (loop.index0 > ns.last_query_index) %}
            {{- '<|im_start|>' + message.role + '\n<think>\n' + reasoning_content + '\n</think>\n\n' + content }}
        {%- else %}
            {{- '<|im_start|>' + message.role + '\n' + content }}
        {%- endif %}
        {%- if message.tool_calls and message.tool_calls is iterable and message.tool_calls is not mapping %}
            {%- for tool_call in message.tool_calls %}
                {%- if tool_call.function is defined %}
                    {%- set tool_call = tool_call.function %}
                {%- endif %}
                {%- if loop.first %}
                    {%- if content|trim %}
                        {{- '\n\n<tool_call>\n<function=' + tool_call.name + '>\n' }}
                    {%- else %}
                        {{- '<tool_call>\n<function=' + tool_call.name + '>\n' }}
                    {%- endif %}
                {%- else %}
                    {{- '\n<tool_call>\n<function=' + tool_call.name + '>\n' }}
                {%- endif %}
                {%- if tool_call.arguments is mapping %}
                    {%- for args_name in tool_call.arguments %}
                        {%- set args_value = tool_call.arguments[args_name] %}
                        {{- '<parameter=' + args_name + '>\n' }}
                        {%- set args_value = args_value | tojson | safe if args_value is mapping or (args_value is sequence and args_value is not string) else args_value | string %}
                        {{- args_value }}
                        {{- '\n</parameter>\n' }}
                    {%- endfor %}
                {%- endif %}
                {{- '</function>\n</tool_call>' }}
            {%- endfor %}
        {%- endif %}
        {{- '<|im_end|>\n' }}
    {%- elif message.role == "tool" %}
        {%- if loop.previtem and loop.previtem.role != "tool" %}
            {{- '<|im_start|>user' }}
        {%- endif %}
        {{- '\n<tool_response>\n' }}
        {{- content }}
        {{- '\n</tool_response>' }}
        {%- if not loop.last and loop.nextitem.role != "tool" %}
            {{- '<|im_end|>\n' }}
        {%- elif loop.last %}
            {{- '<|im_end|>\n' }}
        {%- endif %}
    {%- endif %}
    {%- endif %}
{%- endfor %}
{%- if add_generation_prompt %}
    {{- '<|im_start|>assistant\n' }}
    {%- if enable_thinking is defined and enable_thinking is false %}
        {{- '<think>\n\n</think>\n\n' }}
    {%- else %}
        {{- '<think>\n' }}
    {%- endif %}
{%- endif %}
{#- Unsloth fixes - developer role, tool calling #}`

// qwen36Normalizer is a VERBATIM MIRROR of the ReasoningNormalizer that
// parsers/qwen installs on this model. parsers/qwen imports sdk/kronk/model, so
// an in-package (package model) test cannot import it without an import cycle.
//
//	sdk/kronk/parsers/qwen/reasoning.go:14-22   delegates both methods to
//	sdk/kronk/parsers/standard/reasoning.go:15-18  (closedThink / emptyThink)
//	sdk/kronk/parsers/standard/reasoning.go:24-29  StripThinkContent
//	sdk/kronk/parsers/standard/reasoning.go:34-36  StripEmptyThink
//	sdk/kronk/parsers/standard/reasoning.go:43-62  StripExceptTrailing
//
// MAINTAINER: keep this mirror in sync with those two files.
type qwen36Normalizer struct{}

var (
	qwen36ClosedThink = regexp.MustCompile(`(?s)<think>.*?</think>`)
	qwen36EmptyThink  = regexp.MustCompile(`(?s)<think>\s*</think>`)
)

func (qwen36Normalizer) Name() string                  { return "qwen" }
func (qwen36Normalizer) NewStateMachine() StateMachine { return nil }

func (qwen36Normalizer) ToolCall(_ context.Context, _ applog.Logger, _ string) []ResponseToolCall {
	return nil
}

// StripReasoningContent mirrors standard.StripThinkContent.
func (qwen36Normalizer) StripReasoningContent(content string) string {
	if !strings.Contains(content, "<think>") {
		return content
	}
	return qwen36ClosedThink.ReplaceAllString(content, "")
}

// StripEmptyReasoning mirrors standard.StripEmptyThink -> StripExceptTrailing.
func (qwen36Normalizer) StripEmptyReasoning(rendered string) string {
	locs := qwen36EmptyThink.FindAllStringIndex(rendered, -1)
	if len(locs) == 0 {
		return rendered
	}

	var b strings.Builder
	prev := 0
	for _, loc := range locs {
		start, end := loc[0], loc[1]
		if strings.TrimSpace(rendered[end:]) == "" {
			continue
		}
		b.WriteString(rendered[prev:start])
		prev = end
	}
	b.WriteString(rendered[prev:])

	return b.String()
}

// newQwen36Model builds a *Model wired to the real embedded Qwen3.6 template and
// the qwen reasoning normalizer. bos_token/eos_token are supplied by the caller
// so applyJinjaTemplate (prompts.go:155-164) never reaches into a nil vocab.
func newQwen36Model(imc bool) *Model {
	return &Model{
		cfg:      Config{PtrIncrementalCache: new(imc)},
		template: Template{FileName: "tokenizer.chat_template", Script: qwen36ChatTemplate},
		parser:   qwen36Normalizer{},
		log:      noopLog,
	}
}

// renderQwen36 runs the production request path that shapes reasoning in the
// prompt: normalizeHistoryReasoning (reasoning.go:46, called from chat.go:254)
// followed by applyJinjaTemplate (prompts.go:111, which applies the
// StripEmptyReasoning post-pass at prompts.go:177-181).
func renderQwen36(t *testing.T, m *Model, d D) string {
	t.Helper()

	d["bos_token"] = ""
	d["eos_token"] = "<|im_end|>"

	d = m.normalizeHistoryReasoning(d)

	out, err := m.applyJinjaTemplate(context.Background(), d)
	if err != nil {
		t.Fatalf("applyJinjaTemplate: %v", err)
	}

	return out
}

// TestQwen36EmptyThinkScaffoldStrippedFromInTurnAssistant pins the following
// finding: on every request where an assistant message sits after the last real
// user query — i.e. every turn of a tool-call loop, and every request whose last
// message is an assistant turn — Kronk rewrites that turn's
// "<think>\n\n</think>\n\n" header into two bare newlines, so the model is fed
// "<|im_start|>assistant\n\n\n..." instead of the shape it was trained on.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/prompts.go:177-181     post-render StripEmptyReasoning pass
//   - sdk/kronk/parsers/standard/reasoning.go:34-36 / :43-62 (mirrored above)
//
// # REFERENCE
//
// The model's own embedded chat template, qwen36ChatTemplate L103-104, emits
//
//	'<|im_start|>' + role + '\n<think>\n' + reasoning_content + '\n</think>\n\n' + content
//
// for any assistant turn with loop.index0 > ns.last_query_index. "<think>" and
// "</think>" are single special tokens in this model's vocabulary (248068 /
// 248069), so this header is a fixed two-token frame, not decorative text.
// llama.cpp renders the template output unmodified (common/chat.cpp:887-941 has
// no post-render rewriting of any kind), so llama.cpp feeds the frame through.
//
// # FAILURE SCENARIO
//
// Agentic turn: [user, assistant(tool_calls), tool]. ns.last_query_index is 0
// (the tool result renders as a <tool_response> user turn, which the L79-84 scan
// skips), so the assistant turn at index 1 is rendered with the reasoning frame.
// With no reasoning_content on the message the frame is empty, StripEmptyReasoning
// deletes it because it is not the trailing generation marker, and the assistant
// turn degenerates to "<|im_start|>assistant\n\n\n<tool_call>". The stripping is
// gated only on IncrementalCache() (reasoning.go:36-38), which is on by default,
// so no client cooperation is needed to reach it.
func TestQwen36EmptyThinkScaffoldStrippedFromInTurnAssistant(t *testing.T) {
	msgs := func() []D {
		return []D{
			{"role": "user", "content": "Q1"},
			{
				"role":    "assistant",
				"content": "",
				"tool_calls": []D{
					{"type": "function", "function": D{"name": "f", "arguments": map[string]any{"a": float64(1)}}},
				},
			},
			{"role": "tool", "content": "R"},
		}
	}

	// Reference render: identical inputs with the post-render pass disabled
	// (IMC off), i.e. exactly what the template — and llama.cpp — produce.
	want := renderQwen36(t, newQwen36Model(false), D{
		"messages":        msgs(),
		"enable_thinking": true,
	})

	got := renderQwen36(t, newQwen36Model(true), D{
		"messages":        msgs(),
		"enable_thinking": true,
	})

	const frame = "<|im_start|>assistant\n<think>\n\n</think>\n\n<tool_call>"

	if !strings.Contains(want, frame) {
		t.Fatalf("bad test setup: reference render does not contain the template's reasoning frame\nreference:\n%q", want)
	}

	if got != want {
		t.Errorf("StripEmptyReasoning rewrote the in-turn assistant header.\n"+
			"got:\n%q\nwant:\n%q\n\n"+
			"prompts.go:177-181 runs the parser's StripEmptyReasoning over the FULL rendered\n"+
			"prompt. standard.StripExceptTrailing keeps only a span that sits at the very end\n"+
			"of the string (the generation marker), so the empty <think>\\n\\n</think> frame that\n"+
			"qwen36ChatTemplate L103-104 emits for this assistant turn is deleted, leaving the\n"+
			"two bare newlines that followed it. The model sees\n"+
			"  <|im_start|>assistant\\n\\n\\n<tool_call>\n"+
			"instead of the trained\n"+
			"  <|im_start|>assistant\\n<think>\\n\\n</think>\\n\\n<tool_call>\n"+
			"i.e. special tokens 248068/248069 are replaced by ordinary newline text on every\n"+
			"tool-loop turn.", got, want)
	}
}

// TestQwen36InTurnReasoningContentNotReplayed pins the following finding: Kronk
// deletes reasoning_content from EVERY assistant history message, including the
// turns whose reasoning the model's own template deliberately replays.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/reasoning.go:91-92  (delete "reasoning" / "reasoning_content")
//   - sdk/kronk/model/reasoning.go:36-38  (gated only on IncrementalCache())
//
// # REFERENCE
//
// qwen36ChatTemplate L103-104 replays reasoning_content for any assistant turn
// with loop.index0 > ns.last_query_index. That is the multi-step tool-calling
// case: the reasoning that justified the tool call is part of the SAME logical
// turn as the tool result that follows it, and Qwen3.6 is trained to see it.
// llama.cpp hands inputs.messages to the template verbatim
// (common/chat.cpp:895-900) and has no reasoning-stripping pass anywhere in
// common/chat.cpp, so the replay happens there.
//
// # FAILURE SCENARIO
//
// Turn 2 of a tool loop: [user, assistant(reasoning_content="I must call f
// because X", tool_calls), tool]. The template would render
// "<|im_start|>assistant\n<think>\nI must call f because X\n</think>\n\n<tool_call>...".
// normalizeHistoryReasoning deletes the field first, so the model resumes the
// loop having lost its own justification, then re-derives a different one from
// the tool result — the "I already said that / this is a contradiction" failure
// mode. Kronk's doc comment (reasoning.go:8-18) assumes reasoning is always
// dropped by the template once a newer turn arrives; for this template that is
// only true for turns BEFORE the last user query.
func TestQwen36InTurnReasoningContentNotReplayed(t *testing.T) {
	const reasoning = "I must call f because X"

	msgs := func() []D {
		return []D{
			{"role": "user", "content": "Q1"},
			{
				"role":              "assistant",
				"content":           "",
				"reasoning_content": reasoning,
				"tool_calls": []D{
					{"type": "function", "function": D{"name": "f", "arguments": map[string]any{"a": float64(1)}}},
				},
			},
			{"role": "tool", "content": "R"},
		}
	}

	got := renderQwen36(t, newQwen36Model(true), D{
		"messages":        msgs(),
		"enable_thinking": true,
	})

	// Sanity: the template really does replay it when Kronk leaves the field
	// alone (IMC off skips both normalizeHistoryReasoning and the post-pass).
	want := renderQwen36(t, newQwen36Model(false), D{
		"messages":        msgs(),
		"enable_thinking": true,
	})
	if !strings.Contains(want, "<think>\n"+reasoning+"\n</think>") {
		t.Fatalf("bad test setup: template did not replay in-turn reasoning\nreference:\n%q", want)
	}

	if !strings.Contains(got, reasoning) {
		t.Errorf("in-turn assistant reasoning was dropped from the rendered prompt.\n"+
			"got:\n%q\nwant it to contain:\n%q\n\n"+
			"reasoning.go:91-92 deletes reasoning_content from every assistant message\n"+
			"whenever IncrementalCache() is on and preserve_thinking is off\n"+
			"(reasoning.go:36-38). qwen36ChatTemplate L103-104 replays reasoning for any\n"+
			"assistant turn after the last real user query, which is every turn of a\n"+
			"tool-call loop, and llama.cpp passes reasoning_content straight through\n"+
			"(common/chat.cpp:895-900). The model therefore resumes a multi-step turn with\n"+
			"its own justification missing.", got, reasoning)
	}
}

// classifiedTokenRouting is a VERBATIM MIRROR of the accumulate-then-stream
// ordering inside (*batchEngine).handleSampledToken:
//
//	sdk/kronk/model/batchgen_tokens.go:166-181  channel -> flag transitions
//	sdk/kronk/model/batchgen_tokens.go:200-203  ChannelNone short-circuit
//	sdk/kronk/model/batchgen_tokens.go:212-222  write into the FINAL accumulators
//	sdk/kronk/model/batchgen_tokens.go:224-238  then decide whether to STREAM
//
// The delta routing (reasoning vs content) mirrors chatResponseDelta /
// forContent / forReasoning at sdk/kronk/model/models.go:884-885 and :894-908.
// handleSampledToken itself samples from a live llama context, so the routing is
// mirrored here; isUnnecessaryCRLF is the real production function.
//
// primed reflects batchgen_slot_start.go:52, which sets reasonFlag = 1 when the
// rendered prompt already opened a reasoning block (qwen36ChatTemplate L155
// force-opens "<think>\n").
//
// MAINTAINER: keep this mirror in sync with batchgen_tokens.go:166-238.
func classifiedTokenRouting(primed bool, results []Result) (finalReasoning, finalContent, streamedReasoning, streamedContent string) {
	var m Model

	var reasonFlag, completionFlag, toolFlag int
	if primed {
		reasonFlag = 1
	}

	var fr, fc, sr, sc strings.Builder

	for _, result := range results {
		switch result.Channel {
		case ChannelReasoning:
			reasonFlag++
			completionFlag = 0
			toolFlag = 0

		case ChannelAnswer:
			completionFlag++
			reasonFlag = 0
			toolFlag = 0

		case ChannelTool:
			toolFlag++
			reasonFlag = 0
			completionFlag = 0
		}

		if result.Channel == ChannelNone {
			continue
		}

		switch {
		case reasonFlag > 0:
			fr.WriteString(result.Content)
		case toolFlag > 0:
			// finalTooling: not part of the streamed message.
		default:
			fc.WriteString(result.Content)
		}

		if toolFlag == 0 {
			if m.isUnnecessaryCRLF(reasonFlag, completionFlag, result.Content) {
				continue
			}
			switch {
			case reasonFlag > 0:
				sr.WriteString(result.Content)
			default:
				sc.WriteString(result.Content)
			}
		}
	}

	return fr.String(), fc.String(), sr.String(), sc.String()
}

// TestQwen36StreamedAndFinalMessageAgree pins the following finding: the
// mode-transition newline suppression is applied only to the streamed delta,
// after the same text has already been written to the final accumulator, so a
// streaming client and a non-streaming client receive different assistant
// messages for the identical token sequence.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/batchgen_tokens.go:212-222 (write to finalContent/finalReasoning)
//   - sdk/kronk/model/batchgen_tokens.go:225-230 (isUnnecessaryCRLF -> skip STREAM only)
//   - sdk/kronk/model/model.go:1215-1227        (isUnnecessaryCRLF)
//
// # REFERENCE
//
// llama.cpp never streams anything other than a diff of the accumulated parsed
// message: server_slot::update_chat_msg re-parses the whole generated text and
// derives the deltas with common_chat_msg_diff::compute_diffs
// (tools/server/server-task.cpp:1021 and :177), so the concatenation of the
// streamed deltas is equal to the final message by construction. There is no
// path in llama.cpp that can put a character in one and not the other.
//
// # FAILURE SCENARIO
//
// Qwen3.6 with thinking on: the prompt ends with a force-opened "<think>\n"
// (qwen36ChatTemplate L155) so the slot is primed with reasonFlag = 1
// (batchgen_slot_start.go:52). The model emits reasoning, the "</think>" special
// token (248069, classified ChannelNone), the "\n\n" separator, then the answer.
// The separator is written to finalContent and then suppressed from the stream,
// so Chat() returns content "\n\nB" while ChatStreaming() delivers "B". Any
// caller that stores streamed output as conversation history and later compares
// or re-sends it against a non-streamed turn sees two different assistant turns
// for the same generation.
func TestQwen36StreamedAndFinalMessageAgree(t *testing.T) {
	tests := []struct {
		name    string
		primed  bool
		results []Result
	}{
		{
			// Force-opened <think> (enable_thinking=true, the default).
			name:   "forced-open think, separator after </think>",
			primed: true,
			results: []Result{
				{Channel: ChannelReasoning, Content: "A"},
				{Channel: ChannelNone}, // "</think>" consumed by the state machine
				{Channel: ChannelAnswer, Content: "\n\n"},
				{Channel: ChannelAnswer, Content: "B"},
			},
		},
		{
			// Model emitted "<think>" itself: the first reasoning token is a
			// newline, which isUnnecessaryCRLF drops from the stream only.
			name:   "model-emitted think, leading newline in reasoning",
			primed: false,
			results: []Result{
				{Channel: ChannelNone}, // "<think>" consumed by the state machine
				{Channel: ChannelReasoning, Content: "\n"},
				{Channel: ChannelReasoning, Content: "A"},
				{Channel: ChannelNone}, // "</think>"
				{Channel: ChannelAnswer, Content: "B"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finalReasoning, finalContent, streamedReasoning, streamedContent := classifiedTokenRouting(tt.primed, tt.results)

			if streamedContent != finalContent {
				t.Errorf("streamed content != final content\nstreamed: %q\nfinal:    %q\n\n"+
					"batchgen_tokens.go:212-222 writes result.Content into finalContent BEFORE\n"+
					"batchgen_tokens.go:225-230 asks isUnnecessaryCRLF whether to stream it, so the\n"+
					"suppressed mode-transition newline survives in the non-streaming message only.\n"+
					"llama.cpp streams only diffs of the accumulated parsed message\n"+
					"(tools/server/server-task.cpp:1021 -> common_chat_msg_diff::compute_diffs at\n"+
					":177), so its deltas concatenate to the final message by construction.",
					streamedContent, finalContent)
			}

			if streamedReasoning != finalReasoning {
				t.Errorf("streamed reasoning != final reasoning\nstreamed: %q\nfinal:    %q\n\n"+
					"Same ordering defect as above, on the reasoning channel: the leading newline of\n"+
					"the reasoning block is accumulated into finalReasoning and then dropped from the\n"+
					"reasoning deltas.",
					streamedReasoning, finalReasoning)
			}
		})
	}
}
