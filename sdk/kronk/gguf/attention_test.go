package gguf

import "testing"

func TestCountSWALayers(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    int64
	}{
		{
			name:    "gemma4-60-layers",
			pattern: "[true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false true true true true true false]",
			want:    50,
		},
		{
			name:    "all-true",
			pattern: "[true true true]",
			want:    3,
		},
		{
			name:    "all-false",
			pattern: "[false false false]",
			want:    0,
		},
		{
			name:    "empty",
			pattern: "[]",
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CountSWALayers(tt.pattern)
			if got != tt.want {
				t.Errorf("CountSWALayers() = %d, want %d", got, tt.want)
			}
		})
	}
}
