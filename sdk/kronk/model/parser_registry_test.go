package model

import (
	"context"
	"encoding/json"
	"reflect"
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
