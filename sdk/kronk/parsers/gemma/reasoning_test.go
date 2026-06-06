package gemma

import "testing"

func TestStripReasoningContent(t *testing.T) {
	var p Parser

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no markup", "just an answer", "just an answer"},
		{"empty span", "<|channel>thought\n<channel|>answer", "answer"},
		{"non-empty span", "<|channel>thought\nreasoning\n<channel|>final", "final"},
		{
			name:  "multiple spans",
			input: "<|channel>thought\na\n<channel|>X<|channel>thought\nb\n<channel|>Y",
			want:  "XY",
		},
		{"surrounding text preserved", "before<|channel>thought\nr\n<channel|>after", "beforeafter"},
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
		{"none", "<|turn>user\nhi", "<|turn>user\nhi"},
		{
			name:  "empty history span removed",
			input: "<|turn>model\n<|channel>thought\n<channel|>answer",
			want:  "<|turn>model\nanswer",
		},
		{
			name:  "non-empty span preserved",
			input: "<|turn>model\n<|channel>thought\nreasoned\n<channel|>answer",
			want:  "<|turn>model\n<|channel>thought\nreasoned\n<channel|>answer",
		},
		{
			name:  "trailing generation marker preserved",
			input: "history\n<|turn>model\n<|channel>thought\n<channel|>",
			want:  "history\n<|turn>model\n<|channel>thought\n<channel|>",
		},
		{
			name:  "empty removed but trailing kept",
			input: "<|channel>thought\n<channel|>mid<|turn>model\n<|channel>thought\n<channel|>\n",
			want:  "mid<|turn>model\n<|channel>thought\n<channel|>\n",
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
