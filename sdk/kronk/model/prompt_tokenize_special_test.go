package model

import (
	"context"
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// =============================================================================
// Prompt assembly / tokenization audit for Qwen3.6-35B-A3B-MTP.
//
// Everything in this file is anchored on the real target GGUF:
//
//	/Users/florin/.kronk/models/unsloth/Qwen3.6-35B-A3B-MTP-GGUF/
//	    mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf
//
// Facts read straight out of its metadata (GGUF v3, 55 KV entries):
//
//	tokenizer.ggml.model          = "gpt2"          -> LLAMA_VOCAB_TYPE_BPE
//	tokenizer.ggml.pre            = "qwen35"
//	tokenizer.ggml.add_bos_token  = false           (GGUF_TYPE_BOOL)
//	tokenizer.ggml.add_eos_token  = <absent>
//	tokenizer.ggml.bos_token_id   = 248044          "<|endoftext|>"
//	tokenizer.ggml.eos_token_id   = 248046          "<|im_end|>"
//	token 248045 "<|im_start|>"   token_type 3      (CONTROL)
//	token 248046 "<|im_end|>"     token_type 3      (CONTROL)
//	token 248068 "<think>"        token_type 4      (USER_DEFINED)
//	token 248069 "</think>"       token_type 4      (USER_DEFINED)
//	token 248058 "<tool_call>"    token_type 4      (USER_DEFINED)
//
// Two of the five audit questions came back clean and are recorded here so
// nobody re-opens them:
//
//   - BOS duplication: add_bos_token=false in the GGUF, so
//     model.go:299-302 resolves addBOSToken=false and llama.cpp's
//     llama_vocab::add_bos is false too (llama-vocab.cpp:1813 default, the
//     LLAMA_VOCAB_TYPE_BPE branch never assigns it, and the GGUF override at
//     llama-vocab.cpp:2547-2556 sets it to false). No BOS is prepended by
//     either implementation and the ChatML template emits no bos_token, so
//     there is no double BOS on this model. Continuation prefills are also
//     safe: batchgen_slot_start.go:794 gates addBOS on cacheIdx==0, and the
//     IMC token-plan tail (caching_imc_tokens.go:24-26) is carved out of a
//     single tokenization of the complete render.
//
//   - parse_special: every production tokenize call site passes
//     parseSpecial=true (prompt_plan.go:38, chat.go:306-307, tokenize.go:55,
//     batchgen_slot_start.go:813 and :992), which matches what llama.cpp's
//     server passes for a templated chat prompt —
//     server-context.cpp:4132 and :2273 both call
//     tokenize_input_prompts(vocab, mctx, prompt, /*add_special*/ true,
//     /*parse_special*/ true). <|im_start|>/<|im_end|> therefore land as
//     CONTROL tokens 248045/248046 rather than literal text.
//
// What follows are the divergences that did NOT come back clean.
// =============================================================================

// qwen36GGUFChatTemplate is the VERBATIM tokenizer.chat_template value embedded
// in mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf (8057 bytes,
// sha256 55d4931433fe502b794226ee7f4d206a6bdd436ac9f80eb7d8ebb4c639f9ea0c).
//
// This is the template Kronk actually runs for this model: retrieveTemplate
// (model.go:1032) only reaches the GGUF fallback at model.go:1053 because
// cfg.JinjaFile is unset for the catalog entry and no
// <base>/jinja/Qwen3.6-35B-A3B*.jinja override exists.
//
// It is embedded rather than read from disk because the model file is not part
// of the repository and .extras/ is gitignored.
const qwen36GGUFChatTemplate = `{%- set image_count = namespace(value=0) %}
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

// -----------------------------------------------------------------------------
// Verbatim mirror of the Qwen reasoning normalizer.
//
// prompts.go:178 type-asserts m.parser to ReasoningNormalizer and calls
// StripEmptyReasoning on the fully rendered prompt. The real implementation for
// this model is sdk/kronk/parsers/qwen/reasoning.go:20 delegating to
// sdk/kronk/parsers/standard/reasoning.go:34 -> :43. Those packages import
// sdk/kronk/model, so a test inside package model cannot import them without an
// import cycle. The two regexes and StripExceptTrailing below are copied
// verbatim from:
//
//	sdk/kronk/parsers/standard/reasoning.go:15  closedThink
//	sdk/kronk/parsers/standard/reasoning.go:18  emptyThink
//	sdk/kronk/parsers/standard/reasoning.go:24  StripThinkContent
//	sdk/kronk/parsers/standard/reasoning.go:43  StripExceptTrailing
//
// MAINTAINER: if standard/reasoning.go changes, update this mirror in the same
// commit so the pin keeps tracking production behaviour.
// -----------------------------------------------------------------------------

var (
	mirrorClosedThink = regexp.MustCompile(`(?s)<think>.*?</think>`)
	mirrorEmptyThink  = regexp.MustCompile(`(?s)<think>\s*</think>`)
)

// qwenThinkNormalizer is the Parser + ReasoningNormalizer pair Kronk selects for
// this model (parser "qwen").
type qwenThinkNormalizer struct{}

func (qwenThinkNormalizer) Name() string                  { return "qwen" }
func (qwenThinkNormalizer) NewStateMachine() StateMachine { return nil }
func (qwenThinkNormalizer) ToolCall(_ context.Context, _ applog.Logger, _ string) []ResponseToolCall {
	return nil
}

func (qwenThinkNormalizer) StripReasoningContent(content string) string {
	if !strings.Contains(content, "<think>") {
		return content
	}
	return mirrorClosedThink.ReplaceAllString(content, "")
}

func (qwenThinkNormalizer) StripEmptyReasoning(rendered string) string {
	locs := mirrorEmptyThink.FindAllStringIndex(rendered, -1)
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

// newQwen36PromptModel builds the minimum *Model that Model.applyJinjaTemplate
// (prompts.go:111) touches: the compiled template, the parser, the logger and
// cfg.IncrementalCache (read by stripReasoning at reasoning.go:37).
//
// applyJinjaTemplate only reaches m.vocab when bos_token / eos_token are absent
// from the render context (prompts.go:155-164), so every case below supplies
// both and no llama vocab handle is required.
func newQwen36PromptModel(imc bool) *Model {
	return &Model{
		cfg:      Config{PtrIncrementalCache: &imc},
		log:      noopLog,
		template: Template{FileName: "tokenizer.chat_template", Script: qwen36GGUFChatTemplate},
		parser:   qwenThinkNormalizer{},
	}
}

// renderQwen36Prompt renders msgs through the real production entry point,
// Model.applyJinjaTemplate (prompts.go:111) — the same call
// Model.createPrompt -> applyRequestJinjaTemplate makes for a text-only model
// (prompts.go:23).
func renderQwen36Prompt(t *testing.T, imc bool, msgs []D, extra D) string {
	t.Helper()

	d := D{
		"messages": msgs,
		// Supplied by parseParams (params.go:675) on every real request.
		"enable_thinking": true,
		// llama.cpp seeds these from the vocab in
		// common_chat_template_direct_apply_impl (chat.cpp:897-898). This
		// template never reads them; they are here only to keep
		// applyJinjaTemplate away from m.vocab.
		"bos_token": "<|endoftext|>",
		"eos_token": "<|im_end|>",
	}
	for k, v := range extra {
		d[k] = v
	}

	got, err := newQwen36PromptModel(imc).applyJinjaTemplate(context.Background(), d)
	if err != nil {
		t.Fatalf("applyJinjaTemplate: %v", err)
	}

	return got
}

// TestQwen36PostRenderReasoningStripBreaksAssistantTurns pins the post-render
// reasoning strip that Kronk runs over the finished prompt.
//
// FINDING
// Kronk mutates the rendered chat prompt after the template has produced it.
// prompts.go:177-181 calls ReasoningNormalizer.StripEmptyReasoning on the whole
// string, which deletes every empty "<think>...</think>" span except a trailing
// one (sdk/kronk/parsers/standard/reasoning.go:34 -> :43). The Qwen3.6 template
// emits that span as an inseparable unit with its own trailing blank line —
// template line 104 writes
//
//	'<|im_start|>' + role + '\n<think>\n' + reasoning + '\n</think>\n\n' + content
//
// so removing only the "<think>...</think>" bytes leaves the "\n\n" behind and
// the assistant turn header becomes "<|im_start|>assistant\n\n\n". That is a
// role header the model never saw in training: Qwen3.6 assistant turns open
// either with "<|im_start|>assistant\n<think>...</think>\n\n" or with
// "<|im_start|>assistant\n" and nothing else. The reasoning-channel delimiters
// that told the model which of its own earlier bytes were thinking versus answer
// are gone at the same time.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/prompts.go:177-181   (the post-render pass)
//   - sdk/kronk/model/prompts.go:147-149   (preserve_thinking forced to false)
//   - sdk/kronk/parsers/standard/reasoning.go:34,43 (the strip itself)
//
// LLAMA.CPP REFERENCE
// .extras/llama.cpp/common/chat.cpp:926-940. llama.cpp renders the template and
// performs exactly one post-render edit — trimming a leading BOS / trailing EOS
// when the tokenizer is going to add them (chat.cpp:934-939). It never rewrites
// reasoning markup in the prompt. tools/server/server-context.cpp:4132 then
// tokenizes that string as-is with add_special=true, parse_special=true.
//
// FAILURE SCENARIO
// The strip only fires when template line 103's
// "loop.index0 > ns.last_query_index" is true, i.e. on assistant turns that sit
// AFTER the last real user query. That is precisely the agentic tool-calling
// loop the user is running: system, user, assistant(tool_call), tool,
// assistant(tool_call), tool, ... Every assistant turn already in the history
// arrives at the model as "<|im_start|>assistant\n\n\n<tool_call>" instead of
// "<|im_start|>assistant\n<think>\n\n</think>\n\n<tool_call>". The damage
// accumulates with conversation length and shows up as the model losing track of
// what it previously concluded — asserting something and then contradicting it —
// which is exactly the reported symptom and is invisible to a smoke test that
// only greps the answer for a keyword.
//
// The pass is gated on cfg.IncrementalCache() (reasoning.go:37), documented as
// default-true in sdk/tools/defaults/yaml/model_config.yaml:20 and set to true
// by sdk/kronk/tests/testlib/testlib.go:276. Each case re-renders with IMC off
// to prove the template output itself is byte-correct and localize the corruption
// to prompts.go:177-181.
//
// Expected values were produced by rendering the embedded GGUF template with
// Jinja2 3.1.6 under trim_blocks=True, lstrip_blocks=True — the same lexer
// options llama.cpp's own engine defaults to
// (.extras/llama.cpp/common/jinja/lexer.cpp:115,118).
func TestQwen36PostRenderReasoningStripBreaksAssistantTurns(t *testing.T) {
	toolCalls := []D{
		{"function": D{"name": "get_weather", "arguments": D{"city": "Paris"}}},
	}

	tests := []struct {
		name  string
		msgs  []D
		extra D
		want  string
	}{
		{
			// Control: a plain multi-turn chat emits no empty reasoning span
			// at all, so the strip is a no-op and Kronk matches llama.cpp
			// byte for byte. This case passing is what makes the failures
			// below attributable to the strip and not to the Jinja engine.
			name: "control: plain 3-turn chat is byte-exact",
			msgs: []D{
				{"role": "system", "content": "You are helpful."},
				{"role": "user", "content": "u1"},
				{"role": "assistant", "content": "a1"},
				{"role": "user", "content": "u2"},
				{"role": "assistant", "content": "a2"},
				{"role": "user", "content": "u3"},
			},
			want: "<|im_start|>system\nYou are helpful.<|im_end|>\n" +
				"<|im_start|>user\nu1<|im_end|>\n" +
				"<|im_start|>assistant\na1<|im_end|>\n" +
				"<|im_start|>user\nu2<|im_end|>\n" +
				"<|im_start|>assistant\na2<|im_end|>\n" +
				"<|im_start|>user\nu3<|im_end|>\n" +
				"<|im_start|>assistant\n<think>\n",
		},
		{
			name: "agentic: assistant tool-call turn followed by a tool result",
			msgs: []D{
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "weather?"},
				{"role": "assistant", "content": "", "tool_calls": toolCalls},
				{"role": "tool", "content": "sunny"},
			},
			want: "<|im_start|>system\nsys<|im_end|>\n" +
				"<|im_start|>user\nweather?<|im_end|>\n" +
				"<|im_start|>assistant\n<think>\n\n</think>\n\n" +
				"<tool_call>\n<function=get_weather>\n<parameter=city>\nParis\n</parameter>\n</function>\n</tool_call><|im_end|>\n" +
				"<|im_start|>user\n<tool_response>\nsunny\n</tool_response><|im_end|>\n" +
				"<|im_start|>assistant\n<think>\n",
		},
		{
			name: "agentic: assistant turn with prose and a tool call",
			msgs: []D{
				{"role": "system", "content": "sys"},
				{"role": "user", "content": "weather?"},
				{"role": "assistant", "content": "Let me check.", "tool_calls": toolCalls},
				{"role": "tool", "content": "sunny"},
			},
			want: "<|im_start|>system\nsys<|im_end|>\n" +
				"<|im_start|>user\nweather?<|im_end|>\n" +
				"<|im_start|>assistant\n<think>\n\n</think>\n\nLet me check.\n\n" +
				"<tool_call>\n<function=get_weather>\n<parameter=city>\nParis\n</parameter>\n</function>\n</tool_call><|im_end|>\n" +
				"<|im_start|>user\n<tool_response>\nsunny\n</tool_response><|im_end|>\n" +
				"<|im_start|>assistant\n<think>\n",
		},
		{
			// This is the render chat.go:298 asks for when IMC is on: the
			// same messages with add_generation_prompt=false, used as the
			// cacheable prefix. The corrupted assistant header therefore also
			// ends up in the KV cache and is reused by every later turn.
			name: "imc stable render of a trailing assistant turn",
			msgs: []D{
				{"role": "user", "content": "Hi"},
				{"role": "assistant", "content": "Hello!"},
			},
			extra: D{"add_generation_prompt": false},
			want: "<|im_start|>user\nHi<|im_end|>\n" +
				"<|im_start|>assistant\n<think>\n\n</think>\n\nHello!<|im_end|>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renderQwen36Prompt(t, true, tt.msgs, tt.extra)
			if got == tt.want {
				return
			}

			unstripped := renderQwen36Prompt(t, false, tt.msgs, tt.extra)

			t.Errorf("rendered prompt does not match the llama.cpp reference render\n"+
				"got  (IncrementalCache on):  %q\n"+
				"want (llama.cpp / Jinja2):   %q\n"+
				"same render, IncrementalCache off: %q\n\n"+
				"The template output is correct; prompts.go:177-181 rewrites it afterwards.\n"+
				"StripEmptyReasoning (sdk/kronk/parsers/standard/reasoning.go:34,43) deletes the\n"+
				"empty <think></think> span but not the '\\n\\n' the template emits as part of the\n"+
				"same literal (template line 104), so the assistant header degrades to\n"+
				"\"<|im_start|>assistant\\n\\n\\n\" and the reasoning-channel delimiters vanish.\n"+
				"llama.cpp never edits the rendered prompt except to de-duplicate BOS/EOS\n"+
				"(.extras/llama.cpp/common/chat.cpp:934-939) and tokenizes it verbatim\n"+
				"(tools/server/server-context.cpp:4132).\n"+
				"Fix: drop the post-render pass and let the template's own\n"+
				"preserve_thinking / last_query_index logic decide, as llama.cpp does.",
				got, tt.want, unstripped)
		})
	}
}

// TestQwen36PostRenderReasoningStripDeletesUserContent pins the same
// post-render pass deleting bytes out of a USER message.
//
// FINDING
// StripEmptyReasoning is applied to the entire rendered prompt
// (prompts.go:179), not to assistant turns. Its regex
// `(?s)<think>\s*</think>` (sdk/kronk/parsers/standard/reasoning.go:18) has no
// idea which role block it is inside, so a user who writes an empty
// "<think></think>" pair — asking about the markup, pasting a template, quoting
// a transcript — has those bytes silently removed from the prompt the model
// sees. Nothing in the request path escapes or protects user content before the
// pass runs.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/prompts.go:177-181
//   - sdk/kronk/parsers/standard/reasoning.go:18,43
//
// LLAMA.CPP REFERENCE
// .extras/llama.cpp/common/chat.cpp:926-940 — the rendered prompt is handed to
// the tokenizer unchanged apart from BOS/EOS de-duplication; user content is
// never rewritten. The templated string is then tokenized with parse_special =
// true (tools/server/server-context.cpp:4132), which is also what Kronk does,
// so the control tokens are not the issue here — the deletion is.
//
// FAILURE SCENARIO
// A coding agent asks "why is <think>\n</think> showing up in my output?" and
// the model receives "why is  showing up in my output?". The question is
// mutilated, the answer is nonsense, and no log records the edit.
func TestQwen36PostRenderReasoningStripDeletesUserContent(t *testing.T) {
	msgs := []D{
		{"role": "user", "content": "explain <think>\n</think> markers"},
	}

	const want = "<|im_start|>user\nexplain <think>\n</think> markers<|im_end|>\n" +
		"<|im_start|>assistant\n<think>\n"

	got := renderQwen36Prompt(t, true, msgs, nil)
	if got == want {
		return
	}

	t.Errorf("user message content was rewritten by the post-render reasoning strip\n"+
		"got:  %q\n"+
		"want: %q\n"+
		"same render, IncrementalCache off: %q\n\n"+
		"prompts.go:179 runs StripEmptyReasoning over the WHOLE prompt, so the\n"+
		"`(?s)<think>\\s*</think>` regex at sdk/kronk/parsers/standard/reasoning.go:18\n"+
		"matches inside a user turn and deletes it. llama.cpp hands the rendered\n"+
		"prompt to the tokenizer untouched (.extras/llama.cpp/common/chat.cpp:926-940).\n"+
		"Fix: drop the post-render pass; if empty reasoning spans must be removed,\n"+
		"do it on assistant message content before the render, where\n"+
		"normalizeHistoryReasoning (reasoning.go:46) already operates.",
		got, want, renderQwen36Prompt(t, false, msgs, nil))
}

// TestAddBOSTokenUsesTheVocabFlag pins Kronk's BOS decision being re-derived
// from a GGUF metadata STRING instead of being read from the vocab.
//
// FINDING
// NewModel decides whether to prepend BOS like this (model.go:297-302):
//
//	addBOSToken := true
//	if v, ok := modelInfo.Metadata["tokenizer.ggml.add_bos_token"]; ok && v == "false" {
//	    addBOSToken = false
//	}
//
// That is a re-implementation of a decision llama.cpp already made and exposes,
// and it differs from it in both directions:
//
//   - Key absent. Kronk defaults to true. llama.cpp's llama_vocab::add_bos
//     starts false (llama-vocab.cpp:1813) and only the SPM and WPM branches
//     raise it (llama-vocab.cpp:2374-2384); the LLAMA_VOCAB_TYPE_BPE branch
//     never touches it. A gpt2/BPE GGUF that omits the key — which the Qwen and
//     Llama families are — gets a spurious BOS at position 0 from Kronk and none
//     from llama.cpp.
//
//   - Key present and false. Kronk always honours it. llama.cpp overrides it
//     back to true for Gemma 4 (llama-vocab.cpp:2563-2568, "workaround for
//     Gemma 4"), so on those models Kronk omits a BOS llama.cpp requires.
//     .extras/templates/gemma4.jinja shows Kronk targets that family.
//
// Kronk already has the correct accessor wired up and uses it elsewhere:
// prompt_plan.go:35 passes llama.VocabGetAddBOS(vocab) (yzma
// .extras/yzma/pkg/llama/vocab.go:321 -> llama_vocab_get_add_bos) into the
// media prompt plan, while the text path uses m.addBOSToken. When the two
// disagree the media and text IMC identities for one model disagree too.
//
// PRODUCTION LINES PINNED
//   - sdk/kronk/model/model.go:297-302  (the metadata-string derivation)
//   - sdk/kronk/model/model.go:336      (stored as Model.addBOSToken)
//   - consumers: tokenize.go:55, chat.go:306-307, embed.go:136,193,
//     rerank.go:158,211, batchgen_slot_start.go:794,987
//   - sdk/kronk/model/prompt_plan.go:35 (the path that does ask the vocab)
//
// LLAMA.CPP REFERENCE
// .extras/llama.cpp/src/llama-vocab.cpp:1813 (default false),
// :2374-2394 (per-tokenizer-type defaults), :2547-2556 (GGUF override),
// :2563-2568 (Gemma 4 force-on), :4203-4205 (llama_vocab_get_add_bos).
// The server never re-derives the flag: it calls common_tokenize with
// add_special = true (tools/server/server-context.cpp:4132) and lets
// llama_vocab::add_bos decide (common/common.cpp:1731 ->
// llama-vocab.cpp:3455).
//
// FAILURE SCENARIO
// A spurious BOS at position 0 of every prompt on a BPE GGUF that omits the
// key, or a missing BOS on Gemma 4. Either one is a malformed prompt for the
// whole conversation and degrades coherence diffusely rather than visibly.
//
// This is verified against the source rather than at runtime because the
// derivation is inline in NewModel, which needs a dlopen'd libllama and a
// loaded GGUF. NOTE: it is latent on the reported model —
// mtp-Qwen3.6-35B-A3B-UD-Q8_K_XL.gguf does carry
// tokenizer.ggml.add_bos_token=false, so both implementations agree there.
func TestAddBOSTokenUsesTheVocabFlag(t *testing.T) {
	path := filepath.Join(kronkRepoRoot(t), "sdk", "kronk", "model", "model.go")

	fset := token.NewFileSet()
	f := parseKronkSource(t, fset, path)
	fn := findKronkFunc(t, f, path, "NewModel")

	var readsMetadataKey bool
	var readsVocabFlag bool

	ast.Inspect(fn, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BasicLit:
			if node.Kind == token.STRING && strings.Contains(node.Value, "tokenizer.ggml.add_bos_token") {
				readsMetadataKey = true
			}

		case *ast.SelectorExpr:
			if pkg, ok := node.X.(*ast.Ident); ok && pkg.Name == "llama" && node.Sel.Name == "VocabGetAddBOS" {
				readsVocabFlag = true
			}
		}

		return true
	})

	if readsVocabFlag {
		return
	}

	t.Errorf("NewModel derives addBOSToken without consulting llama_vocab_get_add_bos "+
		"(reads the %q metadata string: %v)\n\n"+
		"model.go:299 starts from `addBOSToken := true` and only lowers it when the GGUF\n"+
		"string is exactly \"false\". llama.cpp's llama_vocab::add_bos starts FALSE\n"+
		"(.extras/llama.cpp/src/llama-vocab.cpp:1813) and is raised only for SPM/WPM\n"+
		"(:2374-2384), so a gpt2/BPE GGUF with no add_bos_token key gets a BOS from Kronk\n"+
		"and none from llama.cpp. In the other direction llama.cpp forces add_bos back on\n"+
		"for Gemma 4 (:2563-2568), which Kronk's string check cannot see.\n"+
		"llama.cpp's server never re-derives this: it passes add_special=true\n"+
		"(tools/server/server-context.cpp:4132) and lets the vocab decide\n"+
		"(common/common.cpp:1731 -> llama-vocab.cpp:3455, llama-vocab.cpp:4203).\n"+
		"Kronk already calls the right accessor on the media path (prompt_plan.go:35),\n"+
		"so the two paths can disagree for the same model.\n"+
		"Fix: set Model.addBOSToken from llama.VocabGetAddBOS(vocab) "+
		"(.extras/yzma/pkg/llama/vocab.go:321).",
		"tokenizer.ggml.add_bos_token", readsMetadataKey)
}
