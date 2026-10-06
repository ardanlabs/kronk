package nemotron

import (
	"encoding/json"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		fp   model.Fingerprint
		want bool
	}{
		{name: "Super architecture", fp: model.Fingerprint{Architecture: "nemotron_h_moe"}, want: true},
		{name: "mixed-case architecture", fp: model.Fingerprint{Architecture: "Nemotron_H_MoE"}, want: true},
		{name: "model name fallback", fp: model.Fingerprint{ModelName: "NVIDIA-Nemotron-3-Super-120B-A12B"}, want: true},
		{name: "other Nemotron H architecture", fp: model.Fingerprint{Architecture: "nemotron_h"}},
		{name: "other architecture", fp: model.Fingerprint{Architecture: "qwen35moe"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, got := New(tt.fp)
			if got != tt.want {
				t.Fatalf("New() matched = %t, want %t", got, tt.want)
			}
			if got && parser.Name() != name {
				t.Errorf("Name() = %q, want %q", parser.Name(), name)
			}
		})
	}
}

func TestParserReasoningAndAnswer(t *testing.T) {
	state := Parser{}.NewStateMachine()

	for _, step := range []struct {
		token   string
		channel model.Channel
		content string
	}{
		{token: "<think>"},
		{token: "reason", channel: model.ChannelReasoning, content: "reason"},
		{token: "</think>"},
		{token: "answer", channel: model.ChannelAnswer, content: "answer"},
	} {
		got, eog := state.Classify(step.token)
		if eog {
			t.Fatalf("Classify(%q) returned EOG", step.token)
		}
		if got.Channel != step.channel || got.Content != step.content {
			t.Errorf("Classify(%q) = %+v, want channel %d content %q", step.token, got, step.channel, step.content)
		}
	}
}

func TestParserThinkingDisabledStartsInAnswer(t *testing.T) {
	state := Parser{}.NewStateMachine()

	got, eog := state.Classify("direct answer")
	if eog {
		t.Fatal("Classify() returned EOG")
	}
	if got.Channel != model.ChannelAnswer || got.Content != "direct answer" {
		t.Errorf("Classify() = %+v, want answer content %q", got, "direct answer")
	}
}

func TestParserToolCallWithSchema(t *testing.T) {
	tools := []model.D{{
		"type": "function",
		"function": model.D{
			"name": "lookup",
			"parameters": model.D{
				"type": "object",
				"properties": model.D{
					"limit": model.D{"type": "integer"},
				},
			},
		},
	}}

	calls := Parser{}.ToolCallWithSchema(t.Context(), nil, `<function=lookup>
<parameter=limit>
3
</parameter>
</function>`, tools)
	if len(calls) != 1 {
		t.Fatalf("ToolCallWithSchema() returned %d calls, want 1", len(calls))
	}
	if calls[0].Function.Name != "lookup" {
		t.Errorf("Function.Name = %q, want %q", calls[0].Function.Name, "lookup")
	}
	if got := calls[0].Function.Arguments["limit"]; got != json.Number("3") {
		t.Errorf("limit = %#v, want json.Number(%q)", got, "3")
	}
}
