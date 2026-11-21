package inkling_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers/inkling"
)

const (
	header = "<|message_model|>"
	think  = "<|content_thinking|>"
	text   = "<|content_text|>"
	invoke = "<|content_invoke_tool_json|>"
	end    = "<|end_message|>"
	eog    = "<|content_model_end_sampling|>"
)

func frame(name, body string) string { return header + name + invoke + body + end }

func collect(t *testing.T, sm parsers.StateMachine, chunks []string, vocabEnd string) map[parsers.Channel]string {
	t.Helper()
	output := make(map[parsers.Channel]string)
	for i, chunk := range chunks {
		if chunk == "" {
			continue
		}
		r, ended := sm.Classify(chunk)
		output[r.Channel] += r.Content
		if ended && strings.Join(chunks[i+1:], "") != "" {
			t.Fatalf("premature EOG at chunk %d", i)
		}
	}
	if vocabEnd != "" {
		sm.(parsers.VocabEOGConsumer).ConsumeVocabEOG(vocabEnd)
	}
	flusher := sm.(parsers.StateMachineFlusher)
	for r := flusher.Flush(); r != (parsers.Result{}); r = flusher.Flush() {
		output[r.Channel] += r.Content
	}
	return output
}

func TestSelection(t *testing.T) {
	for _, tc := range []struct {
		name string
		fp   parsers.Fingerprint
		want bool
	}{
		{"inkling2", parsers.Fingerprint{Architecture: "inkling2"}, true},
		{"loop", parsers.Fingerprint{Architecture: "inkling2_loop"}, true},
		{"case-insensitive", parsers.Fingerprint{Architecture: "INKLING2_LOOP"}, true},
		{"protocol", parsers.Fingerprint{ChatTemplate: header + text + eog}, true},
		{"name-only", parsers.Fingerprint{ModelName: "Inkling-2-Mini"}, false},
		{"partial-protocol", parsers.Fingerprint{ChatTemplate: header + text}, false},
		{"harmony", parsers.Fingerprint{ChatTemplate: "<|channel|><|constrain|>json"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, got := inkling.New(tc.fp); got != tc.want {
				t.Errorf("selection: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestStreamingChannels(t *testing.T) {
	wire := header + think + "Think café." + end + header + text + "Answer." + end +
		header + text + " More <|end_messX|> text." + end + eog
	want := map[parsers.Channel]string{
		parsers.ChannelReasoning: "Think café.", parsers.ChannelAnswer: "Answer. More <|end_messX|> text.",
	}
	for split := range len(wire) + 1 {
		t.Run(fmt.Sprint(split), func(t *testing.T) {
			got := collect(t, inkling.Parser{}.NewStateMachine(), []string{wire[:split], wire[split:]}, "")
			if got[parsers.ChannelReasoning] != want[parsers.ChannelReasoning] || got[parsers.ChannelAnswer] != want[parsers.ChannelAnswer] || got[parsers.ChannelTool] != "" {
				t.Fatalf("channels: got %+v, want %+v", got, want)
			}
		})
	}
	// Classify must stream real content without waiting for the block to close.
	sm := inkling.Parser{}.NewStateMachine()
	r, ended := sm.Classify(header + think + "Plan")
	if r.Channel != parsers.ChannelReasoning || r.Content != "Plan" || ended {
		t.Fatalf("live reasoning: got %+v, EOG %v", r, ended)
	}
	r, ended = sm.Classify(end + header + text + "Answer")
	if r.Channel != parsers.ChannelAnswer || r.Content != "Answer" || ended {
		t.Fatalf("live answer: got %+v, EOG %v", r, ended)
	}
	r, ended = sm.Classify(end + eog)
	if r.Content != "" || !ended {
		t.Fatalf("turn end: got %+v, EOG %v", r, ended)
	}
}

func TestStreamingTools(t *testing.T) {
	// Every structural marker, escaped quotes, slashes, UTF-8 and nested JSON
	// remain data inside JSON strings, even when split one byte at a time.
	const literal = `café "quoted" \ <|message_model|><|content_thinking|><|content_text|><|content_invoke_tool_json|><|end_message|><|content_model_end_sampling|>`
	var encoded strings.Builder
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(literal); err != nil {
		t.Fatal(err)
	}
	first := frame("write.file", `{"name":"write.file","args":{"text":`+strings.TrimSpace(encoded.String())+`,"id":9007199254740993,"nested":{"items":[true,null,2]}}}`)
	second := frame("ping", `{"args":{},"name":"ping"}`)
	wire := header + think + "Plan." + end + first + second + eog
	check := func(t *testing.T, chunks []string, vocab string) {
		t.Helper()
		got := collect(t, inkling.Parser{}.NewStateMachine(), chunks, vocab)
		if got[parsers.ChannelReasoning] != "Plan." || got[parsers.ChannelAnswer] != "" {
			t.Fatalf("channels: got %+v, want reasoning and tools", got)
		}
		calls := (inkling.Parser{}).ToolCall(t.Context(), nil, got[parsers.ChannelTool])
		if len(calls) != 2 || calls[0].Status != 0 || calls[1].Status != 0 {
			t.Fatalf("calls: got %+v, want two valid calls", calls)
		}
		if calls[0].Function.Name != "write.file" || calls[1].Function.Name != "ping" || calls[0].Index != 0 || calls[1].Index != 1 {
			t.Errorf("call names and indices: got %+v", calls)
		}
		want := parsers.ToolCallArguments{
			"text": literal, "id": json.Number("9007199254740993"),
			"nested": map[string]any{"items": []any{true, nil, json.Number("2")}},
		}
		if !reflect.DeepEqual(calls[0].Function.Arguments, want) || len(calls[1].Function.Arguments) != 0 {
			t.Errorf("arguments: got %#v, want %#v", calls[0].Function.Arguments, want)
		}
		if calls[0].ID == "" || calls[0].ID == calls[1].ID || calls[0].Type != "function" {
			t.Errorf("call identities: got %+v", calls)
		}
		// The public argument contract encodes an object inside a JSON string,
		// not an Inkling args envelope or a double-encoded input string.
		wireArgs, err := json.Marshal(calls[0].Function.Arguments)
		if err != nil {
			t.Fatal(err)
		}
		var object string
		if err := json.Unmarshal(wireArgs, &object); err != nil || !json.Valid([]byte(object)) {
			t.Fatalf("wire arguments: got %s, %v, want JSON object string", wireArgs, err)
		}
	}
	for split := range len(wire) + 1 {
		t.Run(fmt.Sprint(split), func(t *testing.T) { check(t, []string{wire[:split], wire[split:]}, "") })
	}
	var bytes []string
	for i := range len(wire) {
		bytes = append(bytes, wire[i:i+1])
	}
	check(t, bytes, "")
	check(t, []string{strings.TrimSuffix(wire, eog)}, eog)
	check(t, []string{strings.TrimSuffix(wire, eog)}, "")
	// Also accept the open model header used by Inkling generation prefixes.
	got := collect(t, inkling.Parser{}.NewStateMachine(), []string{strings.TrimPrefix(second, header) + eog}, "")
	if calls := (inkling.Parser{}).ToolCall(t.Context(), nil, got[parsers.ChannelTool]); len(calls) != 1 || calls[0].Status != 0 {
		t.Fatalf("open header: got %+v", calls)
	}
}

func TestMalformedToolsAreAtomic(t *testing.T) {
	valid := frame("ping", `{"name":"ping","args":{}}`)
	for _, tc := range []struct {
		name string
		wire string
	}{
		{"name-mismatch", frame("ping", `{"name":"other","args":{}}`)},
		{"empty-name", frame("", `{"name":"","args":{}}`)},
		{"name-not-string", frame("ping", `{"name":7,"args":{}}`)},
		{"missing-name", frame("ping", `{"args":{}}`)},
		{"missing-args", frame("ping", `{"name":"ping"}`)},
		{"null-args", frame("ping", `{"name":"ping","args":null}`)},
		{"array-args", frame("ping", `{"name":"ping","args":[]}`)},
		{"string-args", frame("ping", `{"name":"ping","args":"{}"}`)},
		{"arguments-not-args", frame("ping", `{"name":"ping","arguments":{}}`)},
		{"duplicate-name", frame("ping", `{"name":"other","name":"ping","args":{}}`)},
		{"duplicate-args", frame("ping", `{"name":"ping","args":{},"args":{}}`)},
		{"truncated-json", frame("ping", `{"name":"ping","args":{`)},
		{"unterminated-string", frame("ping", `{"name":"ping","args":{"s":"broken`)},
		{"invalid-json", frame("ping", `{"name":"ping","args":{"x":}}`)},
		{"extra-json", frame("ping", `{"name":"ping","args":{}} {}`)},
		{"missing-end", strings.TrimSuffix(valid, end)},
		{"eog-without-message-end", strings.TrimSuffix(valid, end) + eog},
		{"partial-header", header + "ping<|content_invoke_tool_"},
		{"trailing-garbage", valid + "garbage"},
		{"after-eog", valid + eog + "garbage"},
		{"tool-after-eog", valid + eog + valid},
		{"valid-then-invalid", valid + frame("ping", `{"name":"other","args":{}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, chunks := range [][]string{{tc.wire}, strings.Split(tc.wire, "")} {
				// after-eog cases intentionally violate the caller's stop
				// contract, so exercise those as one coalesced piece only.
				if strings.Contains(tc.wire, eog) && len(chunks) > 1 {
					continue
				}
				got := collect(t, inkling.Parser{}.NewStateMachine(), chunks, "")
				calls := (inkling.Parser{}).ToolCall(t.Context(), nil, got[parsers.ChannelTool])
				if len(calls) != 1 || calls[0].Status != 2 || calls[0].Raw == "" || calls[0].Error == "" || calls[0].Function.Name != "" || calls[0].Function.Arguments != nil {
					t.Fatalf("calls: got %+v, want one non-executable failure", calls)
				}
			}
		})
	}
}

func TestFlushAndSlotIsolation(t *testing.T) {
	p := inkling.Parser{}
	a, b := p.NewStateMachine(), p.NewStateMachine()
	a.Classify(header + think + "Queued." + end + header + text + "Stale.<|end_mess")
	b.Classify(frame("ping", `{"name":"ping","args":{}}`))
	a.Reset()
	if got := a.(parsers.StateMachineFlusher).Flush(); got != (parsers.Result{}) {
		t.Fatalf("Reset queued output: got %+v, want empty", got)
	}
	got := collect(t, a, []string{header + text + "Fresh <|end_mess"}, "")
	if got[parsers.ChannelAnswer] != "Fresh <|end_mess" || got[parsers.ChannelTool] != "" || got[parsers.ChannelReasoning] != "" {
		t.Fatalf("trailing Flush: got %+v", got)
	}
	got = collect(t, b, nil, eog)
	calls := p.ToolCall(t.Context(), nil, got[parsers.ChannelTool])
	if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "ping" {
		t.Fatalf("other slot after reset: got %+v", calls)
	}
	b.Reset()
	b.Classify(header + "ping" + invoke + `{"name":"ping","args":{"text":"unfinished`)
	b.(parsers.VocabEOGConsumer).ConsumeVocabEOG(eog)
	got = collect(t, b, nil, "")
	calls = p.ToolCall(t.Context(), nil, got[parsers.ChannelTool])
	if len(calls) != 1 || calls[0].Status != 2 {
		t.Fatalf("native EOG on malformed JSON: got %+v", calls)
	}
	b.Reset()
	got = collect(t, b, []string{header + text + "New." + end}, eog)
	if got[parsers.ChannelAnswer] != "New." || got[parsers.ChannelTool] != "" {
		t.Fatalf("reset tool scanner: got %+v", got)
	}
	b.Reset()
	got = collect(t, b, []string{frame("ping", `{"name":"ping","args":{}}`) + eog}, "")
	calls = p.ToolCall(t.Context(), nil, got[parsers.ChannelTool])
	if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "ping" {
		t.Fatalf("reset scanner for next tool: got %+v", calls)
	}
}
