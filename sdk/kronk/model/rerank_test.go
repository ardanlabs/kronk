package model

import "testing"

func TestRerankTokenLimit(t *testing.T) {
	tests := []struct {
		name          string
		batchTokens   int
		contextTokens int
		want          int
	}{
		{"context limits sequence", 2048, 512, 512},
		{"batch limits sequence", 512, 2048, 512},
		{"equal limits", 2048, 2048, 2048},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rerankTokenLimit(tt.batchTokens, tt.contextTokens)
			if got != tt.want {
				t.Errorf("rerankTokenLimit: got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRerankRejectsOperationDuringUnload(t *testing.T) {
	m := Model{modelInfo: ModelInfo{IsRerankModel: true}}
	m.unloaded.Store(true)

	if _, err := m.Rerank(t.Context(), D{"query": "hello", "documents": []string{"world"}}); err == nil {
		t.Fatal("Rerank() error = nil, want unloading error")
	}
	if got := m.activeStreams.Load(); got != 0 {
		t.Errorf("active streams: got %d, want 0", got)
	}
}
