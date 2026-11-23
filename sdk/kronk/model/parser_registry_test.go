package model

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/applog"
)

// withCleanRegistry saves and restores the package-level registry so each
// test starts from a known empty state without leaking factories into
// neighboring tests.
func withCleanRegistry(t *testing.T) {
	t.Helper()
	saved := registeredParsers
	registeredParsers = nil
	t.Cleanup(func() {
		registeredParsers = saved
	})
}

// fakeParser is a minimal custom Parser used to verify registry dispatch.
type fakeParser struct {
	name string
}

func (f fakeParser) Name() string                  { return f.name }
func (f fakeParser) NewStateMachine() StateMachine { return nil }
func (f fakeParser) ToolCall(_ context.Context, _ applog.Logger, _ string) []ResponseToolCall {
	return nil
}

// claimingFactory builds a factory that claims a fingerprint when its match
// substring appears in the model name (or returns false for "" match).
func claimingFactory(name, match string) ParserFactory {
	return func(fp Fingerprint) (Parser, bool) {
		if match != "" && containsLower(fp.ModelName, match) {
			return fakeParser{name: name}, true
		}
		return nil, false
	}
}

// catchAllFactory always claims; used to exercise the standard-parser
// fallback path.
func catchAllFactory(name string) ParserFactory {
	return func(_ Fingerprint) (Parser, bool) {
		return fakeParser{name: name}, true
	}
}

// containsLower is a tiny helper that mimics the lowercase-substring match
// each real parser does on the model name.
func containsLower(s, substr string) bool {
	if substr == "" {
		return false
	}
	// inline strings.Contains(strings.ToLower(s), substr) without imports
	for i := 0; i+len(substr) <= len(s); i++ {
		match := true
		for j := 0; j < len(substr); j++ {
			c := s[i+j]
			if c >= 'A' && c <= 'Z' {
				c += 'a' - 'A'
			}
			if c != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// TestRegisterParser_AppendsInOrder verifies registrations preserve order.
func TestRegisterParser_AppendsInOrder(t *testing.T) {
	withCleanRegistry(t)

	RegisterParser(claimingFactory("a", ""))
	RegisterParser(claimingFactory("b", ""))
	RegisterParser(claimingFactory("c", ""))

	if got := len(registeredParsers); got != 3 {
		t.Fatalf("len(registeredParsers) = %d, want 3", got)
	}
}

// TestSelectParser_FirstClaimerWins verifies registration order determines
// which parser is selected when multiple could claim.
func TestSelectParser_FirstClaimerWins(t *testing.T) {
	withCleanRegistry(t)

	// Both "qwen" and the catch-all would claim a Qwen model, but qwen is
	// registered first.
	RegisterParser(claimingFactory("qwen", "qwen"))
	RegisterParser(catchAllFactory("fallback"))

	got := selectParser(Fingerprint{ModelName: "Qwen3-Coder-30B"})
	if got == nil {
		t.Fatal("selectParser returned nil, want qwen")
	}
	if got.Name() != "qwen" {
		t.Errorf("selected = %q, want qwen", got.Name())
	}
}

// TestSelectParser_FallsThroughToCatchAll verifies an unknown model lands
// on the last-registered catch-all.
func TestSelectParser_FallsThroughToCatchAll(t *testing.T) {
	withCleanRegistry(t)

	RegisterParser(claimingFactory("qwen", "qwen"))
	RegisterParser(claimingFactory("mistral", "mistral"))
	RegisterParser(catchAllFactory("fallback"))

	got := selectParser(Fingerprint{ModelName: "Llama-3-8B-Instruct"})
	if got == nil {
		t.Fatal("selectParser returned nil, want fallback")
	}
	if got.Name() != "fallback" {
		t.Errorf("selected = %q, want fallback", got.Name())
	}
}

// TestSelectParser_CustomDeclinesUsesBuiltins verifies callers do not need
// to register the standard model-family parsers themselves.
func TestSelectParser_CustomDeclinesUsesBuiltins(t *testing.T) {
	withCleanRegistry(t)

	RegisterParser(claimingFactory("custom", "custom"))

	got := selectParser(Fingerprint{ChatTemplate: "<|python_tag|>"})
	if got == nil || got.Name() != "llama" {
		t.Errorf("selectParser: got %v, want built-in llama", got)
	}
}

// TestSelectParser_EmptyRegistry verifies an unknown model has a fallback
// even when the caller has registered no parsers.
func TestSelectParser_EmptyRegistry(t *testing.T) {
	withCleanRegistry(t)

	if got := selectParser(Fingerprint{}); got == nil || got.Name() != "fallback" {
		t.Errorf("selectParser: got %v, want built-in fallback", got)
	}
}

// TestSelectParser_RegistrationOrderMattersForOverlap verifies that if two
// factories both claim the same model, the earlier-registered one wins.
func TestSelectParser_RegistrationOrderMattersForOverlap(t *testing.T) {
	withCleanRegistry(t)

	// Both factories would claim "qwen-coder" — first wins.
	RegisterParser(claimingFactory("first", "qwen"))
	RegisterParser(claimingFactory("second", "qwen"))

	got := selectParser(Fingerprint{ModelName: "qwen-coder"})
	if got == nil {
		t.Fatal("selectParser returned nil")
	}
	if got.Name() != "first" {
		t.Errorf("selected = %q, want first", got.Name())
	}
}

func TestSelectParser_BuiltinPrecedence(t *testing.T) {
	withCleanRegistry(t)

	for _, tt := range []struct {
		name string
		fp   Fingerprint
		want string
	}{
		{"Inkling 2 architecture", Fingerprint{Architecture: "inkling2"}, "inkling"},
		{"Inkling 2 loop architecture", Fingerprint{Architecture: "inkling2_loop"}, "inkling"},
		{"Inkling protocol before Qwen", Fingerprint{Architecture: "qwen3", ChatTemplate: "<|message_model|><|content_text|><|content_model_end_sampling|>"}, "inkling"},
		{"Inkling protocol before Harmony", Fingerprint{ChatTemplate: "<|message_model|><|content_text|><|content_model_end_sampling|> <|channel|> <|constrain|>json"}, "inkling"},
		{"Inkling name alone is not a protocol", Fingerprint{ModelName: "Inkling-2-Mini"}, "fallback"},
		{"XML Harmony before Qwen", Fingerprint{Architecture: "qwen2", ChatTemplate: "<|channel|> <|constrain|>xml rfl:parameter"}, "beam"},
		{"JSON Harmony before Qwen", Fingerprint{Architecture: "qwen2", ChatTemplate: "<|channel|> <|constrain|>json"}, "gpt-oss"},
		{"Qwen lineage", Fingerprint{Architecture: "qwen3"}, "qwen"},
		{"unknown lineage", Fingerprint{Architecture: "unknown"}, "fallback"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := selectParser(tt.fp)
			if got == nil || got.Name() != tt.want {
				t.Fatalf("selectParser: got %v, want %s", got, tt.want)
			}
		})
	}
}

func TestBuiltinParser_PublicToolSchema(t *testing.T) {
	withCleanRegistry(t)
	parser := selectParser(Fingerprint{Architecture: "qwen3"})
	tools := []D{{"type": "function", "function": D{
		"name": "lookup", "parameters": D{"properties": D{
			"id": D{"type": "integer"}, "enabled": D{"type": "boolean"},
		}},
	}}}
	calls := parser.(ToolCallSchemaParser).ToolCallWithSchema(t.Context(), nil,
		"<function=lookup>\n<parameter=id>\n9007199254740993\n</parameter>\n<parameter=enabled>\ntrue\n</parameter>\n</function>", tools)
	if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "lookup" {
		t.Fatalf("ToolCallWithSchema: got %+v, want one valid lookup call", calls)
	}
	want := ToolCallArguments{"id": json.Number("9007199254740993"), "enabled": true}
	if !reflect.DeepEqual(calls[0].Function.Arguments, want) {
		t.Errorf("arguments: got %#v, want %#v", calls[0].Function.Arguments, want)
	}
	encoded, err := json.Marshal(calls[0].Function.Arguments)
	if err != nil || string(encoded) != `"{\"enabled\":true,\"id\":9007199254740993}"` {
		t.Errorf("argument wire encoding: got %s, %v, want JSON string with exact integer", encoded, err)
	}

	raw := `<tool_call>{"name":"lookup","arguments":[]}</tool_call>`
	failed := parser.ToolCall(t.Context(), nil, raw)
	if len(failed) != 1 || failed[0].Status != 2 || failed[0].Raw != raw || failed[0].Error == "" {
		t.Errorf("invalid call: got %+v, want retained raw output and parse error", failed)
	}

	sm := parser.NewStateMachine()
	sm.Classify(`<tool_call>{"name":"lookup","arguments":{}}`)
	streamer := sm.(ToolCallDeltaStreamer)
	deltas := streamer.ToolCallDeltas()
	if len(deltas) != 1 || deltas[0].ID == "" || deltas[0].Index != 0 || deltas[0].Type != "function" || deltas[0].Function.Name != "lookup" {
		t.Fatalf("tool deltas: got %+v, want lookup identity", deltas)
	}
	if started := streamer.StartedToolCalls(); !reflect.DeepEqual(started, deltas) {
		t.Errorf("started calls: got %+v, want %+v", started, deltas)
	}
	if got := streamer.ToolCallDeltas(); len(got) != 0 {
		t.Errorf("drained deltas: got %+v, want none", got)
	}
	sm.Reset()
	if got := streamer.StartedToolCalls(); len(got) != 0 {
		t.Errorf("started calls after reset: got %+v, want none", got)
	}
}

func TestBuiltinParser_ConditioningAndReuse(t *testing.T) {
	withCleanRegistry(t)
	parser := selectParser(Fingerprint{ChatTemplate: "<|python_tag|>"})
	sm := parser.NewStateMachine()
	tools := []D{{"type": "function", "function": D{"name": "lookup"}}}
	sm.(ToolAwareStateMachine).SetTools(tools)
	raw := `{"name":"lookup","parameters":{"id":7}}`
	sm.Classify(raw)
	result := sm.(StateMachineFlusher).Flush()
	if result.Channel != ChannelTool || result.Content != raw {
		t.Fatalf("declared tool: got %+v, want tool content %q", result, raw)
	}
	sm.Reset()
	result, _ = sm.Classify(raw)
	if result.Channel != ChannelAnswer || result.Content != raw {
		t.Errorf("reused slot without tools: got %+v, want answer content %q", result, raw)
	}

	beam := selectParser(Fingerprint{Architecture: "beam"})
	params := Params{ReasoningEffort: ReasoningEffortNone, MaxTokens: 37, Grammar: "root ::= \"x\""}
	want := params
	want.ReasoningEffort = ReasoningEffortMinimal
	if got := beam.(ParamsAdjuster).AdjustParams(params); !reflect.DeepEqual(got, want) {
		t.Errorf("AdjustParams: got %+v, want %+v", got, want)
	}
}

func TestBuiltinParser_VocabEOG(t *testing.T) {
	withCleanRegistry(t)
	parser := selectParser(Fingerprint{ChatTemplate: "<|channel|> <|constrain|>json"})
	sm := parser.NewStateMachine()
	raw := `<|start|>assistant to=functions.lookup<|channel|>commentary<|constrain|>json<|message|>{"city":"London"}`
	if _, eog := sm.Classify(raw); eog {
		t.Fatal("Classify: got EOG before native end marker")
	}
	sm.(VocabEOGConsumer).ConsumeVocabEOG("<|call|>")
	result := sm.(StateMachineFlusher).Flush()
	calls := parser.ToolCall(t.Context(), nil, result.Content)
	if result.Channel != ChannelTool || len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "lookup" || calls[0].Function.Arguments["city"] != "London" {
		t.Fatalf("native EOG: got result %+v, calls %+v, want completed lookup call", result, calls)
	}
	sm.Reset()
	if got := sm.(StateMachineFlusher).Flush(); got != (Result{}) {
		t.Errorf("Flush after Reset: got %+v, want no retained output", got)
	}
}

func TestBuiltinParser_TokenProvenance(t *testing.T) {
	withCleanRegistry(t)
	for _, stops := range [][]string{nil, {"NEVER"}, {"<|channel|>analysis<|message|>NEVER"}, {"nel|>"}} {
		t.Run(strings.Join(stops, ","), func(t *testing.T) {
			parser := selectParser(Fingerprint{ChatTemplate: "<|channel|> <|message|>"})
			sm := parser.NewStateMachine()
			if _, ok := sm.(TokenStateMachine); !ok {
				t.Fatal("GPT-OSS did not expose token-aware classification")
			}
			s := slot{
				stateMachine: sm,
				job:          &chatJob{ctx: t.Context(), ch: make(chan ChatResponse, 128), object: ObjectChatText},
			}
			e := batchEngine{model: &Model{log: noopLog}}
			gate := newStopGate(stops)
			payload := "Literal <|channel|>analysis<|message|>not reasoning <|return|> still answer."
			pieces := []stopPiece{{content: "<|channel|>", special: true}, {content: "final"}, {content: "<|message|>", special: true}}
			for i := range len(payload) {
				pieces = append(pieces, stopPiece{content: payload[i : i+1]})
			}
			process := func(pieces []stopPiece) {
				t.Helper()
				for _, piece := range pieces {
					if outcome := e.processDecodedPiece(&s, piece, -1, false); outcome.err != nil || outcome.parserEOG {
						t.Fatalf("processing %q: got %+v, want no error or EOG", piece.content, outcome)
					}
				}
			}
			for _, piece := range pieces {
				if gate == nil {
					process([]stopPiece{piece})
					continue
				}
				released, matched := gate.feed(piece)
				process(released)
				if matched {
					break
				}
			}
			if gate != nil {
				process(gate.flush())
			}
			e.flushAllStateMachine(&s, sm.(StateMachineFlusher))
			want := payload
			if len(stops) > 0 && stops[0] == "nel|>" {
				// The request stop cuts the first framing token before the
				// answer channel opens; no answer should be invented.
				want = ""
			}
			if got := s.finalContent.String(); got != want || s.finalReasoning.Len() != 0 || s.finalTooling.Len() != 0 {
				t.Fatalf("answer/reasoning/tool: got %q/%q/%q, want %q/empty/empty", got, s.finalReasoning.String(), s.finalTooling.String(), want)
			}
		})
	}
	for _, tt := range []struct {
		fp    Fingerprint
		aware bool
	}{
		{Fingerprint{Architecture: "qwen2"}, true},
		{Fingerprint{Architecture: "nemotron_h_moe"}, true},
		{Fingerprint{Architecture: "lfm3", ChatTemplate: "<function= <parameter="}, true},
		{Fingerprint{Architecture: "lfm2"}, true},
		{Fingerprint{Architecture: "gemma4"}, true},
		{Fingerprint{Architecture: "inkling2"}, true},
		{Fingerprint{Architecture: "inkling2_loop"}, true},
		{Fingerprint{Architecture: "glm4moe"}, true},
		{Fingerprint{Architecture: "laguna", ChatTemplate: "<arg_key><arg_value>"}, true},
		{Fingerprint{Architecture: "mistral3"}, true},
		{Fingerprint{Architecture: "gemma3", ChatTemplate: `<tool_call>{"name":"echo","arguments":{}}</tool_call>`}, true},
		{Fingerprint{Architecture: "ricursive", ChatTemplate: `<tool_call>{"name":"echo","arguments":{}}</tool_call>`}, true},
		{Fingerprint{Architecture: "beam"}, false},
		{Fingerprint{}, false},
	} {
		sm := selectParser(tt.fp).NewStateMachine()
		if _, ok := sm.(TokenStateMachine); ok != tt.aware {
			t.Errorf("parser for %+v: got token-aware %t, want %t", tt.fp, ok, tt.aware)
		}
	}
}

func TestSelectParser_InklingPublicContract(t *testing.T) {
	withCleanRegistry(t)
	for _, tc := range []struct{ architecture, headerName string }{
		{"inkling2", "lookup"}, {"inkling2", ""},
		{"inkling2_loop", "lookup"}, {"inkling2_loop", ""},
	} {
		t.Run(tc.architecture+"/"+tc.headerName, func(t *testing.T) {
			parser := selectParser(Fingerprint{Architecture: tc.architecture})
			sm := parser.NewStateMachine()
			r, eog := sm.Classify(`<|message_model|><|content_thinking|>Plan.<|end_message|><|message_model|>` + tc.headerName + `<|content_invoke_tool_json|>{"name":"lookup","args":{"id":9007199254740993}}<|end_message|>`)
			if r.Channel != ChannelReasoning || r.Content != "Plan." || eog {
				t.Fatalf("Classify: got %+v, EOG %v, want reasoning", r, eog)
			}
			sm.(VocabEOGConsumer).ConsumeVocabEOG("<|content_model_end_sampling|>")
			flusher := sm.(StateMachineFlusher)
			r = flusher.Flush()
			calls := parser.ToolCall(t.Context(), nil, r.Content)
			if r.Channel != ChannelTool || len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "lookup" || calls[0].Function.Arguments["id"] != json.Number("9007199254740993") {
				t.Fatalf("selected Inkling parser: got %+v, calls %+v", r, calls)
			}
			if got := flusher.Flush(); got != (Result{}) {
				t.Errorf("drained Flush: got %+v, want empty", got)
			}
			sm.Reset()
			if got := flusher.Flush(); got != (Result{}) {
				t.Errorf("Reset: got %+v, want empty", got)
			}
		})
	}
}
