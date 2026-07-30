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
			name: "generation model",
			mi: ModelInfo{
				Metadata: map[string]string{"general.architecture": "qwen3"},
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
