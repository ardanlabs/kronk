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
	second := frame("ping", " \n { \"args\": {}, \"name\": \"ping\" } \t")
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
	for _, tc := range []struct{ name, wire string }{
		{"named", wire},
		{"unnamed", strings.ReplaceAll(strings.ReplaceAll(wire, header+"write.file"+invoke, header+invoke), header+"ping"+invoke, header+invoke)},
		{"mixed", strings.ReplaceAll(wire, header+"write.file"+invoke, header+invoke)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wire := tc.wire
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
		})
	}
	// Also accept the open model header used by Inkling generation prefixes.
	for _, tool := range []string{second, frame("", " \n { \"args\": {}, \"name\": \"ping\" } \t")} {
		got := collect(t, inkling.Parser{}.NewStateMachine(), []string{strings.TrimPrefix(tool, header) + eog}, "")
		if calls := (inkling.Parser{}).ToolCall(t.Context(), nil, got[parsers.ChannelTool]); len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "ping" {
			t.Fatalf("open header: got %+v", calls)
		}
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
		{"unnamed-missing-name", frame("", `{"args":{}}`)},
		{"unnamed-null-name", frame("", `{"name":null,"args":{}}`)},
		{"unnamed-name-not-string", frame("", `{"name":7,"args":{}}`)},
		{"unnamed-whitespace-name", frame("", `{"name":"bad name","args":{}}`)},
		{"unnamed-marker-name", frame("", `{"name":"<|content_text|>","args":{}}`)},
		{"unnamed-duplicate-name", frame("", `{"name":"other","name":"ping","args":{}}`)},
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
		{"unnamed-valid-then-invalid", frame("", `{"name":"ping","args":{}}`) + frame("", `{"name":"other","args":[]}`)},
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
	b.Classify(frame("", `{"name":"ping","args":{}}`))
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

type tokenPiece struct {
	content string
	special bool
}

func collectTokens(t *testing.T, sm parsers.StateMachine, pieces []tokenPiece, split int, vocabEnd bool) map[parsers.Channel]string {
	t.Helper()
	output := make(map[parsers.Channel]string)
	state := sm.(parsers.TokenStateMachine)
	for i, piece := range pieces {
		parts := []string{piece.content}
		if !piece.special {
			at := min(split, len(piece.content))
			parts = []string{piece.content[:at], piece.content[at:]}
		}
		for _, part := range parts {
			if part == "" {
				continue
			}
			r, ended := state.ClassifyToken(part, piece.special)
			output[r.Channel] += r.Content
			if ended && (i != len(pieces)-1 || vocabEnd) {
				t.Fatalf("premature EOG from piece %d (%q)", i, part)
			}
		}
	}
	if vocabEnd {
		sm.(parsers.VocabEOGConsumer).ConsumeVocabEOG(eog)
	}
	flusher := sm.(parsers.StateMachineFlusher)
	for r := flusher.Flush(); r != (parsers.Result{}); r = flusher.Flush() {
		output[r.Channel] += r.Content
	}
	return output
}

func TestTokenChannels(t *testing.T) {
	const literal = header + think + text + invoke + end + eog + " café <|end_mess"
	pieces := []tokenPiece{
		{header, true}, {think, true}, {literal, false}, {end, true},
		{header, true}, {text, true}, {literal, false}, {"<|image|>", true}, {end, true},
	}
	sm := inkling.Parser{}.NewStateMachine()
	for split := range len(literal) + 1 {
		for _, nativeEOG := range []bool{false, true} {
			sm.Reset()
			input := append([]tokenPiece(nil), pieces...)
			if !nativeEOG {
				input = append(input, tokenPiece{eog, true})
			}
			got := collectTokens(t, sm, input, split, nativeEOG)
			if got[parsers.ChannelAnswer] != literal+"<|image|>" || got[parsers.ChannelReasoning] != literal || got[parsers.ChannelTool] != "" {
				t.Fatalf("split %d, native EOG %t: got channels %+v", split, nativeEOG, got)
			}
		}
	}
}

func TestTokenTools(t *testing.T) {
	valid := []tokenPiece{{header, true}, {"ping", false}, {invoke, true}, {`{"name":"ping","args":{}}`, false}, {end, true}}
	for _, tc := range []struct {
		name    string
		pieces  []tokenPiece
		wantErr bool
	}{
		{
			name: "named and unnamed parallel calls with literal and special marker arguments",
			pieces: append([]tokenPiece{
				{header, true}, {invoke, true},
				{`{"name":"write","args":{"id":9007199254740993,"s":"café \"quoted\" \\ `, false},
				{header + think + text + invoke + end + eog, false},
				{end, true}, {`"}}`, false}, {end, true},
			}, valid...),
		},
		{name: "literal second frame cannot execute", pieces: append(append([]tokenPiece(nil), valid...), tokenPiece{frame("ping", `{"name":"ping","args":{}}`), false}), wantErr: true},
		{name: "literal turn terminator", pieces: append(append([]tokenPiece(nil), valid...), tokenPiece{eog, false}), wantErr: true},
		{name: "invalid escape before literal marker", pieces: []tokenPiece{{header, true}, {invoke, true}, {`{"name":"ping","args":{"s":"\`, false}, {end, false}, {`"}}`, false}, {end, true}}, wantErr: true},
		{name: "invalid escape before special marker", pieces: []tokenPiece{{header, true}, {invoke, true}, {`{"name":"ping","args":{"s":"\`, false}, {end, true}, {`"}}`, false}, {end, true}}, wantErr: true},
		{name: "native EOG inside unterminated JSON", pieces: []tokenPiece{{header, true}, {invoke, true}, {`{"name":"ping","args":{"s":"unfinished`, false}}, wantErr: true},
		{name: "header name mismatch", pieces: []tokenPiece{{header, true}, {"other", false}, {invoke, true}, {`{"name":"ping","args":{}}`, false}, {end, true}}, wantErr: true},
		{name: "duplicate name", pieces: []tokenPiece{{header, true}, {invoke, true}, {`{"name":"ping","name":"other","args":{}}`, false}, {end, true}}, wantErr: true},
		{name: "nonobject args", pieces: []tokenPiece{{header, true}, {invoke, true}, {`{"name":"ping","args":[]}`, false}, {end, true}}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sm := inkling.Parser{}.NewStateMachine()
			maxLen := 0
			for _, piece := range tc.pieces {
				maxLen = max(maxLen, len(piece.content))
			}
			for split := range maxLen + 1 {
				sm.Reset()
				got := collectTokens(t, sm, tc.pieces, split, true)
				calls := (inkling.Parser{}).ToolCall(t.Context(), nil, got[parsers.ChannelTool])
				if tc.wantErr {
					if len(calls) != 1 || calls[0].Status != 2 || calls[0].Function.Name != "" || calls[0].Function.Arguments != nil || calls[0].Error == "" {
						t.Fatalf("split %d: got calls %+v, want atomic rejection", split, calls)
					}
					continue
				}
				want := parsers.ToolCallArguments{
					"id": json.Number("9007199254740993"),
					"s":  "café \"quoted\" \\ " + header + think + text + invoke + end + eog + end,
				}
				if len(calls) != 2 || calls[0].Status != 0 || calls[1].Status != 0 || calls[0].Function.Name != "write" || calls[1].Function.Name != "ping" || !reflect.DeepEqual(calls[0].Function.Arguments, want) {
					t.Fatalf("split %d: got calls %+v, want write(%#v) and ping", split, calls, want)
				}
			}
		})
	}
	// Each mandatory frame marker must have native identity, not just matching
	// bytes. Do not accept a valid prefix if a later sibling is forged.
	for _, index := range []int{0, 2, 4} {
		pieces := append(append([]tokenPiece(nil), valid...), valid...)
		pieces[len(valid)+index].special = false
		got := collectTokens(t, inkling.Parser{}.NewStateMachine(), pieces, 1, true)
		calls := (inkling.Parser{}).ToolCall(t.Context(), nil, got[parsers.ChannelTool])
		if len(calls) != 1 || calls[0].Status != 2 {
			t.Fatalf("ordinary marker at %d: got calls %+v, want atomic rejection", index, calls)
		}
	}
	// Generation can start after the template's open model header. Both named
	// and unnamed calls must also survive a length/stop-driven Flush.
	for _, name := range []string{"", "ping"} {
		for _, nativeEOG := range []bool{false, true} {
			pieces := []tokenPiece{{name, false}, {invoke, true}, {`{"name":"ping","args":{}}`, false}, {end, true}}
			got := collectTokens(t, inkling.Parser{}.NewStateMachine(), pieces, 1, nativeEOG)
			calls := (inkling.Parser{}).ToolCall(t.Context(), nil, got[parsers.ChannelTool])
			if len(calls) != 1 || calls[0].Status != 0 || calls[0].Function.Name != "ping" {
				t.Fatalf("open header %q, native EOG %t: got calls %+v", name, nativeEOG, calls)
			}
		}
	}
}
