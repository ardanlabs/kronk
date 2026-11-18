package llama

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/parsers"
)

func testTools() []parsers.D {
	return []parsers.D{{
		"type": "function",
		"function": parsers.D{
			"name": "get_weather",
			"parameters": parsers.D{
				"type": "object",
			},
		},
	}}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		fp   parsers.Fingerprint
		want bool
	}{
		{"JSON template", parsers.Fingerprint{Architecture: "llama", ChatTemplate: `Respond in the format {"name": function name, "parameters": dictionary}`}, true},
		{"python tag", parsers.Fingerprint{ChatTemplate: `<|python_tag|>`}, true},
		{"architecture alone", parsers.Fingerprint{Architecture: "llama"}, false},
		{"unrelated", parsers.Fingerprint{Architecture: "gemma3"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, got := New(tt.fp)
			if got != tt.want {
				t.Errorf("New: got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBareJSONRequiresMatchingDeclaredTool(t *testing.T) {
	tests := []struct {
		name    string
		tools   []parsers.D
		content string
		channel parsers.Channel
		flush   bool
	}{
		{"matching tool", testTools(), `{"name":"get_weather","parameters":{"location":"NYC"}}`, parsers.ChannelTool, true},
		{"unknown tool", testTools(), `{"name":"delete_everything","parameters":{}}`, parsers.ChannelAnswer, false},
		{"no declared tools", nil, `{"name":"get_weather","parameters":{"location":"NYC"}}`, parsers.ChannelAnswer, false},
		{"ordinary JSON", testTools(), `{"forecast":"sunny"}`, parsers.ChannelAnswer, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &stateMachine{status: parsers.ChannelAnswer}
			sm.SetTools(tt.tools)
			got, _ := sm.Classify(tt.content)
			if tt.flush {
				if got != (parsers.Result{}) {
					t.Fatalf("Classify: got %+v, want buffered", got)
				}
				got = sm.Flush()
			} else if got == (parsers.Result{}) {
				got = sm.Flush()
			}
			if got.Channel != tt.channel || got.Content != tt.content {
				t.Errorf("Classify: got %+v, want channel %v content %q", got, tt.channel, tt.content)
			}
		})
	}
}

func TestBareJSONAcrossTokens(t *testing.T) {
	sm := &stateMachine{status: parsers.ChannelAnswer}
	sm.SetTools(testTools())

	if got, _ := sm.Classify(`{"name":"get_`); got != (parsers.Result{}) {
		t.Errorf("first token: got %+v, want buffered", got)
	}
	got, _ := sm.Classify(`weather","parameters":{"location":"NYC"}}`)
	if got != (parsers.Result{}) {
		t.Errorf("second token: got %+v, want buffered", got)
	}
	got = sm.Flush()
	if got.Channel != parsers.ChannelTool || got.Content != `{"name":"get_weather","parameters":{"location":"NYC"}}` {
		t.Errorf("Flush: got %+v", got)
	}
}

func TestBareJSONRequiresTheCompleteAnswer(t *testing.T) {
	sm := &stateMachine{status: parsers.ChannelAnswer}
	sm.SetTools(testTools())
	if got, _ := sm.Classify("I will call it: "); got.Channel != parsers.ChannelAnswer {
		t.Fatalf("prose: got %+v", got)
	}
	call := `{"name":"get_weather","parameters":{"location":"NYC"}}`
	if got, _ := sm.Classify(call); got.Channel != parsers.ChannelAnswer || got.Content != call {
		t.Errorf("call after prose: got %+v, want answer", got)
	}

	sm.Reset()
	sm.SetTools(testTools())
	if got, _ := sm.Classify(call); got != (parsers.Result{}) {
		t.Fatalf("candidate: got %+v, want buffered", got)
	}
	got, _ := sm.Classify(" trailing prose")
	if got.Channel != parsers.ChannelAnswer || got.Content != call+" trailing prose" {
		t.Errorf("trailing prose: got %+v, want complete answer", got)
	}
}

func TestLeadingWhitespaceBeforePythonTag(t *testing.T) {
	sm := &stateMachine{status: parsers.ChannelAnswer}
	sm.SetTools(testTools())
	if got, _ := sm.Classify("\n"); got != (parsers.Result{}) {
		t.Fatalf("whitespace: got %+v, want buffered", got)
	}
	call := `{"name":"get_weather","parameters":{"location":"NYC"}}`
	got, _ := sm.Classify(pythonTag + call)
	if got != (parsers.Result{}) {
		t.Errorf("marked call: got %+v, want buffered", got)
	}
	if got := sm.Flush(); got.Channel != parsers.ChannelTool || got.Content != "\n"+pythonTag+call {
		t.Errorf("Flush: got %+v, want complete marked call", got)
	}
}

func TestPythonTagJSON(t *testing.T) {
	sm := &stateMachine{status: parsers.ChannelAnswer}
	sm.SetTools(testTools())
	got, _ := sm.Classify(`<|python_tag|>{"name":"get_weather","parameters":{"location":"NYC"}}`)
	if got != (parsers.Result{}) {
		t.Fatalf("Classify: got %+v, want buffered", got)
	}
	got = sm.Flush()
	if got.Channel != parsers.ChannelTool || got.Content != `<|python_tag|>{"name":"get_weather","parameters":{"location":"NYC"}}` {
		t.Errorf("Flush: got %+v", got)
	}
}

func TestToolCallPreservesJSONTypes(t *testing.T) {
	calls := Parser{}.ToolCall(t.Context(), nil, `{"name":"get_weather","parameters":{"location":"NYC","count":9007199254740993,"ratio":1.50}}`)
	if len(calls) != 1 {
		t.Fatalf("ToolCall: got %d calls, want 1", len(calls))
	}
	want := parsers.ToolCallArguments{
		"location": "NYC",
		"count":    json.Number("9007199254740993"),
		"ratio":    json.Number("1.50"),
	}
	got := calls[0].Function.Arguments
	for name, value := range want {
		if got[name] != value {
			t.Errorf("argument %q: got %#v, want %#v", name, got[name], value)
		}
	}
}

func TestMarkedMalformedCallIsReported(t *testing.T) {
	for _, content := range []string{"not JSON", pythonTag} {
		calls := Parser{}.ToolCall(t.Context(), nil, content)
		if len(calls) != 1 || calls[0].Status != 2 || calls[0].Error == "" {
			t.Errorf("ToolCall(%q): got %+v, want one failed call", content, calls)
		}
	}
}

func TestToolCallRequiresOneStrictEnvelope(t *testing.T) {
	valid := `{"name":"get_weather","parameters":{"locations":[],"nested":{"ok":true}}}`
	tests := []struct {
		name  string
		input string
		ok    bool
	}{
		{"bare", valid, true},
		{"marked", " \n" + pythonTag + "\t" + valid + "\r", true},
		{"prefix", "prose " + valid, false},
		{"suffix", valid + " prose", false},
		{"two objects", valid + valid, false},
		{"malformed tail", valid + `{`, false},
		{"truncated envelope", `{"name":"get_weather","parameters":`, false},
		{"truncated nested object", `{"name":"get_weather","parameters":{"a":{"b":1}`, false},
		{"truncated string", `{"name":"get_weather","parameters":{"a":"x`, false},
		{"parameters array", `{"name":"get_weather","parameters":[]}`, false},
		{"duplicate envelope", `{"name":"get_weather","na\u006de":"other","parameters":{}}`, false},
		{"duplicate parameter", `{"name":"get_weather","parameters":{"city":"a","city":"b"}}`, false},
		{"duplicate nested", `{"name":"get_weather","parameters":{"x":{"key":1,"k\u0065y":2}}}`, false},
		{"empty tag", pythonTag, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := Parser{}.ToolCall(t.Context(), nil, tt.input)
			if len(calls) != 1 {
				t.Fatalf("ToolCall: got %d calls, want 1", len(calls))
			}
			if tt.ok {
				if calls[0].Status != 0 || calls[0].Function.Name != "get_weather" {
					t.Fatalf("ToolCall: got %+v, want success", calls[0])
				}
				locations, ok := calls[0].Function.Arguments["locations"].([]any)
				if !ok || locations == nil || len(locations) != 0 {
					t.Errorf("locations: got %#v, want non-nil empty array", calls[0].Function.Arguments["locations"])
				}
				return
			}
			if calls[0].Status != parseErrorStatus || calls[0].Function.Name != "" || calls[0].Raw != tt.input {
				t.Errorf("ToolCall: got %+v, want atomic failure with full raw input", calls[0])
			}
		})
	}
}

func TestStateMachinePythonTagAtEverySplit(t *testing.T) {
	call := `{"name":"get_weather","parameters":{"text":"literal <think> and ` + pythonTag + `"}}`
	stream := pythonTag + call
	for split := range len(stream) + 1 {
		t.Run(string(rune(split)), func(t *testing.T) {
			sm := &stateMachine{status: parsers.ChannelAnswer}
			sm.SetTools(testTools())
			for _, piece := range []string{stream[:split], stream[split:]} {
				if result, _ := sm.Classify(piece); result != (parsers.Result{}) {
					t.Fatalf("Classify: got %+v, want buffered", result)
				}
			}
			first := sm.Flush()
			second := sm.Flush()
			if first.Channel != parsers.ChannelTool || first.Content != stream || second != (parsers.Result{}) {
				t.Errorf("Flush: got %+v then %+v, want one exact tool result", first, second)
			}
		})
	}
}

func TestStateMachineOneByteThinkMarkers(t *testing.T) {
	stream := `<think>reasoning</think>answer`
	sm := &stateMachine{status: parsers.ChannelAnswer}
	var reasoning, answer strings.Builder
	for _, char := range stream {
		result, _ := sm.Classify(string(char))
		switch result.Channel {
		case parsers.ChannelReasoning:
			reasoning.WriteString(result.Content)
		case parsers.ChannelAnswer:
			answer.WriteString(result.Content)
		}
	}
	for result := sm.Flush(); result != (parsers.Result{}); result = sm.Flush() {
		if result.Channel == parsers.ChannelReasoning {
			reasoning.WriteString(result.Content)
		} else {
			answer.WriteString(result.Content)
		}
	}
	if reasoning.String() != "reasoning" || answer.String() != "answer" {
		t.Errorf("output: got reasoning %q answer %q", reasoning.String(), answer.String())
	}
}

func TestStateMachineCoalescedThinkPreservesSourceOrder(t *testing.T) {
	sm := &stateMachine{status: parsers.ChannelAnswer}
	first, _ := sm.Classify(`<think>r</think>a`)
	second, _ := sm.Classify("b")
	if first.Channel != parsers.ChannelReasoning || first.Content != "r" {
		t.Errorf("first result: got %+v, want reasoning %q", first, "r")
	}
	if second.Channel != parsers.ChannelAnswer || second.Content != "ab" {
		t.Errorf("second result: got %+v, want answer %q", second, "ab")
	}
	if got := sm.Flush(); got != (parsers.Result{}) {
		t.Errorf("Flush: got %+v, want empty", got)
	}
}

func TestStateMachineLeadingWhitespaceAndCoalescedThink(t *testing.T) {
	sm := &stateMachine{status: parsers.ChannelAnswer}
	first, _ := sm.Classify(" \n<think>reason</think>answer")
	second := sm.Flush()
	third := sm.Flush()
	if first.Channel != parsers.ChannelAnswer || first.Content != " \n" {
		t.Errorf("first result: got %+v, want leading whitespace answer", first)
	}
	if second.Channel != parsers.ChannelReasoning || second.Content != "reason" {
		t.Errorf("second result: got %+v, want reasoning", second)
	}
	if third.Channel != parsers.ChannelAnswer || third.Content != "answer" {
		t.Errorf("third result: got %+v, want answer", third)
	}
}

func TestStateMachineThinkThenMarkedCallCoalescedAndSplit(t *testing.T) {
	call := `{"name":"get_weather","parameters":{"location":"NYC"}}`
	stream := `<think>r</think>` + pythonTag + call
	for split := range len(stream) + 1 {
		t.Run(string(rune(split)), func(t *testing.T) {
			sm := &stateMachine{status: parsers.ChannelAnswer}
			sm.SetTools(testTools())
			var results []parsers.Result
			for _, piece := range []string{stream[:split], stream[split:]} {
				if result, _ := sm.Classify(piece); result != (parsers.Result{}) {
					results = append(results, result)
				}
			}
			for result := sm.Flush(); result != (parsers.Result{}); result = sm.Flush() {
				results = append(results, result)
			}
			if len(results) != 2 {
				t.Fatalf("results: got %+v, want reasoning and tool", results)
			}
			if results[0].Channel != parsers.ChannelReasoning || results[0].Content != "r" {
				t.Errorf("first result: got %+v, want reasoning", results[0])
			}
			if results[1].Channel != parsers.ChannelTool || results[1].Content != pythonTag+call {
				t.Errorf("second result: got %+v, want marked tool", results[1])
			}
		})
	}
}

func TestStateMachineFalsePartialPrefixesRetainBytes(t *testing.T) {
	tests := []struct {
		name   string
		pieces []string
		want   parsers.Result
	}{
		{"think open", []string{"<thi", "x>answer"}, parsers.Result{Channel: parsers.ChannelAnswer, Content: "<thix>answer"}},
		{"python tag", []string{"<|python_", "tax|>answer"}, parsers.Result{Channel: parsers.ChannelAnswer, Content: "<|python_tax|>answer"}},
		{"think close", []string{"<think>reason</thi", "x>tail"}, parsers.Result{Channel: parsers.ChannelReasoning, Content: "reason</thix>tail"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &stateMachine{status: parsers.ChannelAnswer}
			var content strings.Builder
			for _, piece := range tt.pieces {
				if result, _ := sm.Classify(piece); result.Channel == tt.want.Channel {
					content.WriteString(result.Content)
				}
			}
			for result := sm.Flush(); result != (parsers.Result{}); result = sm.Flush() {
				if result.Channel == tt.want.Channel {
					content.WriteString(result.Content)
				}
			}
			if content.String() != tt.want.Content {
				t.Errorf("content: got %q, want %q", content.String(), tt.want.Content)
			}
		})
	}
}

func TestMarkedUnknownAndIncomplete(t *testing.T) {
	tests := []struct {
		name    string
		content string
		channel parsers.Channel
	}{
		{"valid", pythonTag + `{"name":"get_weather","parameters":{}}`, parsers.ChannelTool},
		{"unknown", pythonTag + `{"name":"unknown","parameters":{}}`, parsers.ChannelAnswer},
		{"incomplete", pythonTag + `{"name":"get_weather","parameters":{"x":`, parsers.ChannelTool},
		{"prose suffix", pythonTag + `{"name":"get_weather","parameters":{}} prose`, parsers.ChannelTool},
		{"second object", pythonTag + `{"name":"get_weather","parameters":{}}{}`, parsers.ChannelTool},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &stateMachine{status: parsers.ChannelAnswer}
			sm.SetTools(testTools())
			for _, char := range tt.content {
				sm.Classify(string(char))
			}
			got := sm.Flush()
			if got.Channel != tt.channel || got.Content != tt.content || sm.Flush() != (parsers.Result{}) {
				t.Errorf("Flush: got %+v, want channel %v exact content", got, tt.channel)
			}
		})
	}
}
