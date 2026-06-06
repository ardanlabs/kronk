package glm

import "testing"

func TestStripReasoningContent(t *testing.T) {
	var p Parser

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no markup", "just an answer", "just an answer"},
		{"empty span", "<think>\n\n</think>answer", "answer"},
		{"non-empty span", "<think>reasoning here</think>final", "final"},
		{"leading text preserved", "before<think>r</think>after", "beforeafter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.StripReasoningContent(tt.input)
			if got != tt.want {
				t.Errorf("StripReasoningContent: got %q, want %q", got, tt.want)
			}

			if again := p.StripReasoningContent(got); again != got {
				t.Errorf("StripReasoningContent not idempotent: %q -> %q", got, again)
			}
		})
	}
}

func TestStripEmptyReasoning(t *testing.T) {
	var p Parser

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "none",
			input: "<|user|>hi",
			want:  "<|user|>hi",
		},
		{
			name:  "current-block empty span removed",
			input: "<|assistant|><think></think>answer",
			want:  "<|assistant|>answer",
		},
		{
			name:  "prior-turn bare close removed",
			input: "<|assistant|></think>answer<|user|>next",
			want:  "<|assistant|>answer<|user|>next",
		},
		{
			// The same turn must render identically whether it was a
			// current-block (empty span) or prior (bare close) turn.
			name:  "mixed forms collapse to identical content-only",
			input: "<|assistant|><think></think>first<|user|>q<|assistant|></think>second",
			want:  "<|assistant|>first<|user|>q<|assistant|>second",
		},
		{
			name:  "trailing open think marker preserved",
			input: "<|assistant|></think>answer<|user|>q<|assistant|><think>",
			want:  "<|assistant|>answer<|user|>q<|assistant|><think>",
		},
		{
			name:  "trailing bare close marker preserved",
			input: "<|assistant|></think>answer<|user|>q<|assistant|></think>",
			want:  "<|assistant|>answer<|user|>q<|assistant|></think>",
		},
		{
			name:  "non-empty span preserved",
			input: "<|assistant|><think>kept</think>answer<|user|>q",
			want:  "<|assistant|><think>kept</think>answer<|user|>q",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.StripEmptyReasoning(tt.input)
			if got != tt.want {
				t.Errorf("StripEmptyReasoning: got %q, want %q", got, tt.want)
			}

			if again := p.StripEmptyReasoning(got); again != got {
				t.Errorf("StripEmptyReasoning not idempotent: %q -> %q", got, again)
			}
		})
	}
}
