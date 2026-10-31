package models_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestModelPurpose(t *testing.T) {
	tests := []struct {
		name     string
		id       string
		metadata map[string]string
		endpoint string
	}{
		{"MiniLM without filename hint", "all-MiniLM-L6-v2-Q8_0", map[string]string{"general.architecture": "bert", "bert.pooling_type": "1"}, "embeddings"},
		{"Jina without filename hint", "renamed", map[string]string{"general.architecture": "jina-bert-v2", "jina-bert-v2.pooling_type": "1"}, "embeddings"},
		{"CLS pooling overrides rerank name", "rerank", map[string]string{"general.architecture": "bert", "bert.pooling_type": "2"}, "embeddings"},
		{"last pooling", "renamed", map[string]string{"general.architecture": "qwen3", "qwen3.pooling_type": "3"}, "embeddings"},
		{"rank pooling overrides embed name", "embed", map[string]string{"general.architecture": "bert", "bert.pooling_type": "4"}, "rerank"},
		{"BGE rerank wins over embed tags", "bge-reranker-v2-m3-Q8_0", map[string]string{"general.architecture": "bert", "general.name": "Bge M3", "general.tags": "[text-embeddings-inference]"}, "rerank"},
		{"BERT default", "all-MiniLM-L6-v2", map[string]string{"general.architecture": "bert"}, "embeddings"},
		{"filename fallback", "legacy-embed", nil, "embeddings"},
		{"generation with no pooling", "ordinary", map[string]string{"general.architecture": "llama", "llama.pooling_type": "0"}, "chat_completion"},
		{"invalid pooling is not embedding evidence", "ordinary", map[string]string{"general.architecture": "llama", "llama.pooling_type": "invalid"}, "chat_completion"},
		{"decision metadata beats pooling and name", "embed-rerank", map[string]string{"general.architecture": "modern-bert", "modern-bert.decision.type": "laya", "modern-bert.pooling_type": "4"}, "decision"},
		{"legacy decision BERT", "Laya-Q8_0", map[string]string{"general.architecture": "modern-bert"}, "decision"},
		{"legacy decision beats embed hint", "OpenJEV-embed-Q8_0", map[string]string{"general.architecture": "llama"}, "decision"},
		{"Julia decision fallback", "Julia-1-Q8_0", map[string]string{"general.architecture": "modern-bert"}, "decision"},
		{"decision token boundary", "clever-model", map[string]string{"general.architecture": "llama"}, "chat_completion"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := models.CapabilitiesForModel(tt.metadata, false, tt.id)
			if caps.Endpoint != tt.endpoint {
				t.Errorf("endpoint: got %q, want %q", caps.Endpoint, tt.endpoint)
			}
			info := models.ModelInfoFromMetadata(tt.id, tt.metadata, 0, false, false)
			if info.IsEmbedModel != (tt.endpoint == "embeddings") || info.IsRerankModel != (tt.endpoint == "rerank") {
				t.Errorf("model flags: got embed=%t rerank=%t, want endpoint %q", info.IsEmbedModel, info.IsRerankModel, tt.endpoint)
			}
		})
	}
}

func TestCatalogModelDimensions(t *testing.T) {
	tests := []struct {
		name         string
		metadata     map[string]string
		wantType     string
		wantSequence string
	}{
		{"dense attention", map[string]string{"general.architecture": "llama"}, "Dense", "Attention-only"},
		{"moe attention", map[string]string{"general.architecture": "qwen3moe", "qwen3moe.expert_count": "128"}, "MoE", "Attention-only"},
		{"dense hybrid", map[string]string{"general.architecture": "qwen35", "qwen35.block_count": "8"}, "Dense", "Hybrid"},
		{"moe hybrid", map[string]string{"general.architecture": "qwen35moe", "qwen35moe.block_count": "8", "qwen35moe.expert_count": "256"}, "MoE", "Hybrid"},
		{"recurrent only", map[string]string{"general.architecture": "mamba"}, "Dense", "Recurrent"},
		{"all recurrent pattern", map[string]string{"general.architecture": "qwen35", "qwen35.block_count": "3", "qwen35.attention.recurrent_layers": "[true,true,true]"}, "Dense", "Recurrent"},
		{"all attention pattern", map[string]string{"general.architecture": "qwen35", "qwen35.block_count": "3", "qwen35.attention.recurrent_layers": "[false,false,false]"}, "Dense", "Attention-only"},
		{"generic explicit hybrid", map[string]string{"general.architecture": "clef", "clef.block_count": "3", "clef.attention.recurrent_layers": "[true,false,true]"}, "Dense", "Hybrid"},
		{"exclude appended mtp", map[string]string{"general.architecture": "clef", "clef.block_count": "3", "clef.nextn_predict_layers": "1", "clef.attention.recurrent_layers": "[true,true,false]"}, "Dense", "Recurrent"},
		{"unknown recurrent topology", map[string]string{"general.architecture": "custom", "custom.ssm.state_size": "16"}, "Dense", "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := arch.Resolve(tt.metadata)
			if got := profile.ParameterActivation(); got != tt.wantType {
				t.Errorf("profile parameter activation: got %q, want %q", got, tt.wantType)
			}
			if got := profile.SequenceProcessing; got != tt.wantSequence {
				t.Errorf("sequence processing: got %q, want %q", got, tt.wantSequence)
			}
			architecture := tt.metadata["general.architecture"]
			entry := models.CatalogEntry{ModelType: tt.wantType, Architecture: architecture, SequenceProcessing: tt.wantSequence}
			summary := models.NewSummary("test/model", entry, nil, nil)
			if summary.ModelType != tt.wantType || summary.Architecture != architecture || summary.SequenceProcessing != tt.wantSequence {
				t.Errorf("summary dimensions: got %q/%q/%q, want %q/%q/%q", summary.ModelType, summary.Architecture, summary.SequenceProcessing, tt.wantType, architecture, tt.wantSequence)
			}
		})
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{name: "negative", input: -1, want: ""},
		{name: "zero", input: 0, want: ""},
		{name: "bytes", input: 1023, want: "1023 B"},
		{name: "kibibyte", input: 1 << 10, want: "1 KiB"},
		{name: "mebibyte", input: 1 << 20, want: "1 MiB"},
		{name: "gibibyte", input: 1 << 30, want: "1.00 GiB"},
		{name: "fractional gibibyte", input: 3 << 29, want: "1.50 GiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := models.FormatBytes(tt.input); got != tt.want {
				t.Errorf("FormatBytes: got %q, want %q", got, tt.want)
			}
		})
	}
}
