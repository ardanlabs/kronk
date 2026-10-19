package beam_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/beam"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/gpt"
)

func TestSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		fp        model.Fingerprint
		beam, gpt bool
	}{
		{"beam-architecture", model.Fingerprint{Architecture: "beam", ChatTemplate: "<|channel|>"}, true, false},
		{"xml-harmony", model.Fingerprint{Architecture: "qwen2", ChatTemplate: "<|channel|> <|constrain|>xml rfl:parameter"}, true, false},
		{"gpt-oss", model.Fingerprint{Architecture: "qwen2", ChatTemplate: "<|channel|> <|constrain|>json"}, false, true},
		{"json-with-xml-example", model.Fingerprint{ChatTemplate: "<|channel|> <|constrain|>json example rfl:parameter"}, false, true},
		{"unrelated", model.Fingerprint{Architecture: "qwen3"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := beam.New(tc.fp); ok != tc.beam {
				t.Errorf("Beam selection: got %v, want %v", ok, tc.beam)
			}
			if _, ok := gpt.New(tc.fp); ok != tc.gpt {
				t.Errorf("GPT-OSS selection: got %v, want %v", ok, tc.gpt)
			}
		})
	}
}

func frame(name, body, end string) string {
	return "<|start|>assistant to=" + name + "<|channel|>analysis <|constrain|>xml<|message|>" + body + end
}

func parameter(name, value string) string {
	return "<rfl:parameter name=\"" + name + "\">\n" + value + "\n</rfl:parameter>"
}

// stream exercises only public parser interfaces and collects every channel,
// including results queued behind an end-of-generation event.
func stream(t *testing.T, chunks []string, vocabEnd string) map[model.Channel]string {
	t.Helper()
	sm := beam.Parser{}.NewStateMachine()
	output := make(map[model.Channel]string)
	for i, chunk := range chunks {
		if chunk == "" {
			continue
		}
		r, eog := sm.Classify(chunk)
		output[r.Channel] += r.Content
		if eog && strings.Join(chunks[i+1:], "") != "" {
			t.Fatalf("premature EOG at chunk %d", i)
		}
	}
	if vocabEnd != "" {
		sm.(model.VocabEOGConsumer).ConsumeVocabEOG(vocabEnd)
	}
	flusher := sm.(model.StateMachineFlusher)
	for r := flusher.Flush(); r != (model.Result{}); r = flusher.Flush() {
		output[r.Channel] += r.Content
	}
	sm.Reset()
	if got := flusher.Flush(); got != (model.Result{}) {
		t.Fatalf("Reset: got %+v, want no output", got)
	}
	return output
}

func TestStreamingToolCalls(t *testing.T) {
	const raw = "  café & <tag> \\n literal <|call|> text\nsecond line  "
	tools := []model.D{{"type": "function", "function": model.D{
		"name": "write", "parameters": model.D{"type": "object", "properties": model.D{
			"text": model.D{"type": "string"}, "count": model.D{"type": "integer"},
		}},
	}}}
	first := frame("write", parameter("text", raw)+"\n"+parameter("count", "9007199254740993"), "<|end|>")
	second := frame("ping", "", "<|call|>")
	// Beam's prompt already ends in assistant, so generation begins at the
	// channel marker rather than repeating that role prefix.
	wire := "<|channel|>analysis<|message|>Plan carefully.<|end|>" + first + second
	check := func(t *testing.T, chunks []string, end string) {
		t.Helper()
		got := stream(t, chunks, end)
		if got[model.ChannelReasoning] != "Plan carefully." || got[model.ChannelAnswer] != "" {
			t.Fatalf("channels: got %+v, want reasoning only", got)
		}
		calls := (beam.Parser{}).ToolCallWithSchema(t.Context(), nil, got[model.ChannelTool], tools)
		if len(calls) != 2 || calls[0].Status != 0 || calls[1].Status != 0 {
			t.Fatalf("calls: got %+v, want two valid calls", calls)
		}
		if calls[0].Function.Name != "write" || calls[1].Function.Name != "ping" {
			t.Errorf("names: got %q and %q, want write and ping", calls[0].Function.Name, calls[1].Function.Name)
		}
		want := model.ToolCallArguments{"text": raw, "count": json.Number("9007199254740993")}
		if !reflect.DeepEqual(calls[0].Function.Arguments, want) {
			t.Errorf("arguments: got %#v, want %#v", calls[0].Function.Arguments, want)
		}
		if calls[0].ID == "" || calls[0].ID == calls[1].ID {
			t.Errorf("call identities: got %q and %q, want distinct nonempty IDs", calls[0].ID, calls[1].ID)
		}
	}
	for split := range len(wire) + 1 {
		t.Run(fmt.Sprint(split), func(t *testing.T) { check(t, []string{wire[:split], wire[split:]}, "") })
	}
	var chunks []string
	for i := range len(wire) {
		chunks = append(chunks, wire[i:i+1])
	}
	check(t, chunks, "")
	check(t, []string{strings.TrimSuffix(wire, "<|call|>")}, "<|call|>")
}

func TestAnswerAndReasoning(t *testing.T) {
	for _, wire := range []string{
		"<|channel|>analysis<|message|>Think.<|end|><|start|>assistant<|channel|>final<|message|>Answer.<|return|>",
		"<|start|>assistant<|channel|>analysis<|message|>Think.<|end|><|start|>assistant<|channel|>final<|constrain|>json<|message|>Answer.<|return|>",
	} {
		for split := range len(wire) + 1 {
			got := stream(t, []string{wire[:split], wire[split:]}, "")
			if got[model.ChannelReasoning] != "Think." || got[model.ChannelAnswer] != "Answer." || got[model.ChannelTool] != "" {
				t.Fatalf("split %d: got %+v", split, got)
			}
		}
	}
}

func TestParameterTypes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ty      any
		raw     string
		want    any
		wantErr bool
	}{
		{"string-number", "string", "123", "123", false},
		{"string-json", "string", `{"x":1}`, `{"x":1}`, false},
		{"boolean", "boolean", "false", false, false},
		{"number", "number", "-2.75", json.Number("-2.75"), false},
		{"integer-exponent", "integer", "1e3", json.Number("1e3"), false},
		{"large-integer", "integer", "18446744073709551616", json.Number("18446744073709551616"), false},
		{"object", "object", `{"x":[true,2]}`, map[string]any{"x": []any{true, json.Number("2")}}, false},
		{"array", "array", `[1,"two"]`, []any{json.Number("1"), "two"}, false},
		{"null", []any{"string", "null"}, "null", nil, false},
		{"union-number", []string{"string", "number"}, "42", json.Number("42"), false},
		{"union-string", []string{"string", "number"}, "hello", "hello", false},
		{"oneOf", model.D{"oneOf": []any{model.D{"type": "string"}, model.D{"type": "boolean"}}}, "true", true, false},
		{"fraction-not-integer", "integer", "2.5", nil, true},
		{"invalid-boolean", "boolean", "no", nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			schema, ok := tc.ty.(model.D)
			if !ok {
				schema = model.D{"type": tc.ty}
			}
			tools := []model.D{{"type": "function", "function": model.D{"name": "test", "parameters": model.D{"properties": model.D{"value": schema}}}}}
			// Exercise the same JSON decoding and document normalization used
			// by the chat API, not just hand-built Go schema maps.
			wire, err := json.Marshal(model.D{"tools": tools})
			if err != nil {
				t.Fatal(err)
			}
			var request map[string]any
			if err := json.Unmarshal(wire, &request); err != nil {
				t.Fatal(err)
			}
			tools = model.MapToModelD(request)["tools"].([]model.D)
			calls := (beam.Parser{}).ToolCallWithSchema(t.Context(), nil, frame("test", parameter("value", tc.raw), "<|call|>"), tools)
			if len(calls) != 1 || (calls[0].Status != 0) != tc.wantErr {
				t.Fatalf("calls: got %+v, wantErr %v", calls, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(calls[0].Function.Arguments["value"], tc.want) {
				t.Errorf("value: got %#v, want %#v", calls[0].Function.Arguments["value"], tc.want)
			}
		})
	}
}

func TestGenerationEnd(t *testing.T) {
	for _, end := range []string{"<|call|>", "<|return|>", "<|endoftext|>", "<|im_end|>", "<|fim_pad|>", "<|repo_name|>", "<|file_sep|>"} {
		sm := (beam.Parser{}).NewStateMachine()
		r, eog := sm.Classify(frame("ping", "", end))
		if !eog || r.Channel != model.ChannelTool {
			t.Fatalf("end %q: got %+v, eog %v, want tool call and EOG", end, r, eog)
		}
		calls := (beam.Parser{}).ToolCall(t.Context(), nil, r.Content)
		if len(calls) != 1 || calls[0].Status != 0 {
			t.Fatalf("end %q: got %+v, want valid tool call", end, calls)
		}
	}
	for _, wire := range []string{"Answer.", "Answer.<|return|>"} {
		got := stream(t, []string{wire}, "")
		if got[model.ChannelAnswer] != "Answer." {
			t.Errorf("unchanneled answer: got %+v", got)
		}
	}
	for _, effort := range []string{"", "none", "minimal", "low", "medium", "high", "xhigh", "max"} {
		want := effort
		if effort == "none" {
			want = "minimal"
		}
		got := (beam.Parser{}).AdjustParams(model.Params{ReasoningEffort: effort})
		if got.ReasoningEffort != want {
			t.Errorf("effort %q: got %q, want %q", effort, got.ReasoningEffort, want)
		}
	}
}

func TestMalformedAndMissingClosers(t *testing.T) {
	valid := frame("write", parameter("text", "safe"), "<|call|>")
	for _, wire := range []string{
		strings.TrimSuffix(valid, "<|call|>"),
		frame("write", "<rfl:parameter name=\"text\">\nsafe", ""),
		frame("write", parameter("text", "one")+parameter("text", "two"), "<|call|>"),
		frame("write", parameter("text", "one")+"garbage", "<|call|>"),
		strings.Replace(valid, "<|constrain|>xml", "<|constrain|>json", 1),
		valid + "garbage",
		frame("first", "", "<|end|>") + frame("broken", "<rfl:parameter", "<|call|>"),
		" to=write<|channel|>analysis <|constrain|>xml",
	} {
		got := stream(t, []string{wire}, "")
		calls := (beam.Parser{}).ToolCall(t.Context(), nil, got[model.ChannelTool])
		if len(calls) != 1 || calls[0].Status == 0 || calls[0].Function.Name != "" || calls[0].Raw == "" {
			t.Errorf("wire %q: got %+v, want one non-executable failure", wire, calls)
		}
	}
	for _, wire := range []string{
		strings.Replace(valid, "\n</rfl:parameter>", "\n", 1),
		frame("write", "<rfl:parameter name=\"text\">\nsafe\n"+parameter("other", "two"), "<|call|>"),
	} {
		got := stream(t, []string{wire}, "")
		calls := (beam.Parser{}).ToolCall(t.Context(), nil, got[model.ChannelTool])
		if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Arguments["text"] != "safe" {
			t.Errorf("missing parameter closer: got %+v", calls)
		}
	}
}
