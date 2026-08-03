package llama

import (
	"encoding/json"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func testTools() []model.D {
	return []model.D{{
		"type": "function",
		"function": model.D{
			"name": "get_weather",
			"parameters": model.D{
				"type": "object",
			},
		},
	}}
}

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		{"JSON template", model.Fingerprint{Architecture: "llama", ChatTemplate: `Respond in the format {"name": function name, "parameters": dictionary}`}, true},
		{"python tag", model.Fingerprint{ChatTemplate: `<|python_tag|>`}, true},
		{"architecture alone", model.Fingerprint{Architecture: "llama"}, false},
		{"unrelated", model.Fingerprint{Architecture: "gemma3"}, false},
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
		tools   []model.D
		content string
		channel model.Channel
		flush   bool
	}{
		{"matching tool", testTools(), `{"name":"get_weather","parameters":{"location":"NYC"}}`, model.ChannelTool, true},
		{"unknown tool", testTools(), `{"name":"delete_everything","parameters":{}}`, model.ChannelAnswer, false},
		{"no declared tools", nil, `{"name":"get_weather","parameters":{"location":"NYC"}}`, model.ChannelAnswer, false},
		{"ordinary JSON", testTools(), `{"forecast":"sunny"}`, model.ChannelAnswer, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sm := &stateMachine{status: model.ChannelAnswer}
			sm.SetTools(tt.tools)
			got, _ := sm.Classify(tt.content)
			if tt.flush {
				if got != (model.Result{}) {
					t.Fatalf("Classify: got %+v, want buffered", got)
				}
				got = sm.Flush()
			}
			if got.Channel != tt.channel || got.Content != tt.content {
				t.Errorf("Classify: got %+v, want channel %v content %q", got, tt.channel, tt.content)
			}
		})
	}
}

func TestBareJSONAcrossTokens(t *testing.T) {
	sm := &stateMachine{status: model.ChannelAnswer}
	sm.SetTools(testTools())

	if got, _ := sm.Classify(`{"name":"get_`); got != (model.Result{}) {
		t.Errorf("first token: got %+v, want buffered", got)
	}
	got, _ := sm.Classify(`weather","parameters":{"location":"NYC"}}`)
	if got != (model.Result{}) {
		t.Errorf("second token: got %+v, want buffered", got)
	}
	got = sm.Flush()
	if got.Channel != model.ChannelTool || got.Content != `{"name":"get_weather","parameters":{"location":"NYC"}}` {
		t.Errorf("Flush: got %+v", got)
	}
}

func TestBareJSONRequiresTheCompleteAnswer(t *testing.T) {
	sm := &stateMachine{status: model.ChannelAnswer}
	sm.SetTools(testTools())
	if got, _ := sm.Classify("I will call it: "); got.Channel != model.ChannelAnswer {
		t.Fatalf("prose: got %+v", got)
	}
	call := `{"name":"get_weather","parameters":{"location":"NYC"}}`
	if got, _ := sm.Classify(call); got.Channel != model.ChannelAnswer || got.Content != call {
		t.Errorf("call after prose: got %+v, want answer", got)
	}

	sm.Reset()
	sm.SetTools(testTools())
	if got, _ := sm.Classify(call); got != (model.Result{}) {
		t.Fatalf("candidate: got %+v, want buffered", got)
	}
	got, _ := sm.Classify(" trailing prose")
	if got.Channel != model.ChannelAnswer || got.Content != call+" trailing prose" {
		t.Errorf("trailing prose: got %+v, want complete answer", got)
	}
}

func TestLeadingWhitespaceBeforePythonTag(t *testing.T) {
	sm := &stateMachine{status: model.ChannelAnswer}
	sm.SetTools(testTools())
	if got, _ := sm.Classify("\n"); got != (model.Result{}) {
		t.Fatalf("whitespace: got %+v, want buffered", got)
	}
	call := `{"name":"get_weather","parameters":{"location":"NYC"}}`
	got, _ := sm.Classify(pythonTag + call)
	if got.Channel != model.ChannelTool || got.Content != call {
		t.Errorf("marked call: got %+v", got)
	}
	if got := sm.Flush(); got != (model.Result{}) {
		t.Errorf("Flush: got %+v, want empty", got)
	}
}

func TestPythonTagJSON(t *testing.T) {
	sm := &stateMachine{status: model.ChannelAnswer}
	sm.SetTools(testTools())
	got, _ := sm.Classify(`<|python_tag|>{"name":"get_weather","parameters":{"location":"NYC"}}`)
	if got.Channel != model.ChannelTool || got.Content != `{"name":"get_weather","parameters":{"location":"NYC"}}` {
		t.Errorf("Classify: got %+v", got)
	}
}

func TestToolCallPreservesJSONTypes(t *testing.T) {
	calls := Parser{}.ToolCall(t.Context(), nil, `{"name":"get_weather","parameters":{"location":"NYC","count":9007199254740993,"ratio":1.50}}`)
	if len(calls) != 1 {
		t.Fatalf("ToolCall: got %d calls, want 1", len(calls))
	}
	want := model.ToolCallArguments{
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
