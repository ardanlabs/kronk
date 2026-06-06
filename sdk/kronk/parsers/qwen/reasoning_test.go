package qwen

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
		{"multiline span", "<think>\nline1\nline2\n</think>\nout", "\nout"},
		{"multiple spans", "<think>a</think>X<think>b</think>Y", "XY"},
		{"leading text preserved", "before<think>r</think>after", "beforeafter"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.StripReasoningContent(tt.input)
			if got != tt.want {
				t.Errorf("StripReasoningContent: got %q, want %q", got, tt.want)
			}

			// Idempotent.
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
		{"none", "<|im_start|>user\nhi<|im_end|>\n", "<|im_start|>user\nhi<|im_end|>\n"},
		{
			name:  "empty history span removed",
			input: "<|im_start|>assistant\n<think>\n\n</think>\n\nanswer<|im_end|>\n",
			want:  "<|im_start|>assistant\n\n\nanswer<|im_end|>\n",
		},
		{
			name:  "non-empty span preserved",
			input: "<|im_start|>assistant\n<think>\nreasoned\n</think>\n\nanswer<|im_end|>\n",
			want:  "<|im_start|>assistant\n<think>\nreasoned\n</think>\n\nanswer<|im_end|>\n",
		},
		{
			name:  "trailing generation marker preserved",
			input: "history<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n",
			want:  "history<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n",
		},
		{
			name:  "empty removed but trailing kept",
			input: "<think>\n\n</think>mid<|im_start|>assistant\n<think>\n\n</think>\n",
			want:  "mid<|im_start|>assistant\n<think>\n\n</think>\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.StripEmptyReasoning(tt.input)
			if got != tt.want {
				t.Errorf("StripEmptyReasoning: got %q, want %q", got, tt.want)
			}

			// Idempotent.
			if again := p.StripEmptyReasoning(got); again != got {
				t.Errorf("StripEmptyReasoning not idempotent: %q -> %q", got, again)
			}
		})
	}
}
