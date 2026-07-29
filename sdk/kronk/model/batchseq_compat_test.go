package model

import "testing"

func TestSupportsBatchSeq(t *testing.T) {
	tests := []struct {
		name string
		mi   ModelInfo
		want bool
	}{
		{
			name: "Qwen3 embedding",
			mi: ModelInfo{
				IsEmbedModel: true,
				Metadata:     map[string]string{"general.architecture": "qwen3"},
			},
			want: true,
		},
		{
			name: "BERT reranker",
			mi: ModelInfo{
				IsRerankModel: true,
				Metadata:      map[string]string{"general.architecture": "bert"},
			},
			want: true,
		},
		{
			name: "EmbeddingGemma fallback",
			mi: ModelInfo{
				IsEmbedModel: true,
				Metadata:     map[string]string{"general.architecture": "gemma-embedding"},
			},
			want: false,
		},
		{
			name: "unknown embedding fallback",
			mi: ModelInfo{
				IsEmbedModel: true,
				Metadata:     map[string]string{"general.architecture": "unknown"},
			},
			want: false,
		},
		{
			name: "unknown reranker fallback",
			mi: ModelInfo{
				IsRerankModel: true,
				Metadata:      map[string]string{"general.architecture": "unknown"},
			},
			want: false,
		},
		{
			name: "Qwen3 reranker fallback",
			mi: ModelInfo{
				IsRerankModel: true,
				Metadata:      map[string]string{"general.architecture": "qwen3"},
			},
			want: false,
		},
		{
			name: "generation model",
			mi: ModelInfo{
				Metadata: map[string]string{"general.architecture": "qwen3"},
			},
			want: false,
		},
		{
			name: "missing metadata",
			mi: ModelInfo{
				IsEmbedModel: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsBatchSeq(tt.mi); got != tt.want {
				t.Errorf("supportsBatchSeq: got %t, want %t", got, tt.want)
			}
		})
	}
}
