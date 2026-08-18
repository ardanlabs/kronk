package models

import (
	"errors"
	"testing"
)

func TestParseModelID(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    ModelID
		wantErr bool
	}{
		{
			name:  "base model",
			value: "unsloth/Qwen3-0.6B-Q8_0",
			want:  ModelID{Provider: "unsloth", Model: "Qwen3-0.6B-Q8_0"},
		},
		{
			name:  "named configuration",
			value: "unsloth/Qwen3-0.6B-Q8_0/AGENT",
			want:  ModelID{Provider: "unsloth", Model: "Qwen3-0.6B-Q8_0", Profile: "AGENT"},
		},
		{name: "bare model", value: "Qwen3-0.6B-Q8_0", wantErr: true},
		{name: "too many segments", value: "unsloth/Qwen3/AGENT/extra", wantErr: true},
		{name: "empty provider", value: "/Qwen3", wantErr: true},
		{name: "empty model", value: "unsloth/", wantErr: true},
		{name: "empty profile", value: "unsloth/Qwen3/", wantErr: true},
		{name: "padded segment", value: "unsloth/ Qwen3", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseModelID(tt.value)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidModelID) {
					t.Fatalf("ParseModelID() error = %v, want ErrInvalidModelID", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseModelID() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseModelID() = %#v, want %#v", got, tt.want)
			}
			if got.String() != tt.value {
				t.Errorf("String() = %q, want %q", got.String(), tt.value)
			}
		})
	}
}
