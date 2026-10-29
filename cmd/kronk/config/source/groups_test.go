package source

import "testing"

func TestChoiceGroup(t *testing.T) {
	// The catalog's endpoints for these come from the SDK's profile, which
	// calls every BERT model a reranker: the names decide, as at runtime.
	for _, tt := range []struct {
		c    Choice
		want Group
	}{
		{Choice{ID: "unsloth/Qwen3-0.6B-Q8_0"}, GroupChat},
		{Choice{ID: "Qwen/Qwen3-Embedding-0.6B-Q8_0", Endpoint: "embeddings"}, GroupEmbeddings},
		{Choice{ID: "nomic-ai/nomic-embed-text-v1.5.Q8_0", Endpoint: "embeddings"}, GroupEmbeddings},
		{Choice{ID: "gpustack/bge-reranker-v2-m3-Q8_0", Endpoint: "embeddings"}, GroupRerank},
		{Choice{ID: "ggml-org/jina-embeddings-v2-base-en-q8_0", Endpoint: "rerank"}, GroupEmbeddings},
		{Choice{ID: "second-state/all-MiniLM-L6-v2-Q8_0", Endpoint: "rerank"}, GroupEmbeddings},
		{Choice{ID: "a/decider", Endpoint: "decision"}, GroupDecision},
		{Choice{ID: "tiny", Kind: KindAudioModel}, GroupAudio},
		{Choice{ID: "sd-1.5", Kind: KindImageModel}, GroupImage},
	} {
		if got := tt.c.Group(); got != tt.want {
			t.Errorf("%s: group = %s, want %s", tt.c.ID, got, tt.want)
		}
	}
}
