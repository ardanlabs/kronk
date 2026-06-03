package models

import (
	"slices"
	"testing"
)

func Test_fullPathLookupKeys(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    []string
	}{
		{
			name:    "bare model name",
			modelID: "Qwopus3.5-4B-Coder.Q8_0",
			want:    []string{"Qwopus3.5-4B-Coder.Q8_0"},
		},
		{
			name:    "model/variant",
			modelID: "Qwopus3.5-4B-Coder.Q8_0/AGENT",
			want:    []string{"Qwopus3.5-4B-Coder.Q8_0", "AGENT"},
		},
		{
			name:    "org/model",
			modelID: "mradermacher/Qwopus3.5-4B-Coder.Q8_0",
			want:    []string{"mradermacher", "Qwopus3.5-4B-Coder.Q8_0"},
		},
		{
			name:    "org/model/variant prefers bare model in the middle",
			modelID: "mradermacher/Qwopus3.5-4B-Coder.Q8_0/AGENT",
			want:    []string{"Qwopus3.5-4B-Coder.Q8_0", "mradermacher", "AGENT"},
		},
		{
			name:    "org/model/playground/session keeps bare model first",
			modelID: "mradermacher/Qwopus3.5-4B-Coder.Q8_0/playground/sess-1",
			want:    []string{"Qwopus3.5-4B-Coder.Q8_0", "mradermacher", "sess-1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fullPathLookupKeys(tt.modelID)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
