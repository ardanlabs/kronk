package models_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// Expected values for Qwen3-8B-Q8_0.gguf model.
var (
	expDesc = "Qwen3 8B Instruct"

	// Raw metadata key expectations (stored as strings).
	expMetaArchitecture            = "qwen3"
	expMetaGeneralType             = "model"
	expMetaName                    = "Qwen3 8B Instruct"
	expMetaFinetune                = "Instruct"
	expMetaBaseName                = "Qwen3"
	expMetaSizeLabel               = "8B"
	expMetaQuantizationVersion     = "2"
	expMetaFileType                = "7"
	expMetaContextLength           = "40960"
	expMetaEmbeddingLength         = "4096"
	expMetaBlockCount              = "36"
	expMetaFeedForwardLength       = "12288"
	expMetaHeadCount               = "32"
	expMetaHeadCountKV             = "8"
	expMetaLayerNormRMSEpsilon     = "1e-06"
	expMetaAttentionKeyLength      = "128"
	expMetaAttentionValueLength    = "128"
	expMetaRopeFreqBase            = "1e+06"
	expMetaTokenizerModel          = "gpt2"
	expMetaTokenizerPre            = "qwen2"
	expMetaTokenizerEOSTokenID     = "151645"
	expMetaTokenizerPaddingTokenID = "151643"
	expMetaTokenizerBOSTokenID     = "151643"
	expMetaTokenizerAddBOSToken    = "false"
)

func TestModelMetadata(t *testing.T) {
	m, err := models.New()
	if err != nil {
		t.Fatalf("Unable to create models api: %v", err)
	}

	modelID := "Qwen3-8B-Q8_0"

	info, err := m.ModelInformation(modelID)
	if err != nil {
		t.Fatalf("ModelMetadata failed: %v", err)
	}

	// Test ModelInfo struct fields.
	if info.Desc != expDesc {
		t.Errorf("Desc: got %q, want %q", info.Desc, expDesc)
	}

	if info.Size == 0 {
		t.Error("Size should not be zero")
	}
	if info.FileType != 7 {
		t.Errorf("FileType: got %d, want 7", info.FileType)
	}
	if info.Quantization != "Q8_0" {
		t.Errorf("Quantization: got %q, want %q", info.Quantization, "Q8_0")
	}

	if info.HasProjection {
		t.Error("HasProjection should be false when no projection path provided")
	}

	// Test raw metadata values.
	metaTests := []struct {
		key  string
		want string
	}{
		{"general.architecture", expMetaArchitecture},
		{"general.type", expMetaGeneralType},
		{"general.name", expMetaName},
		{"general.finetune", expMetaFinetune},
		{"general.basename", expMetaBaseName},
		{"general.size_label", expMetaSizeLabel},
		{"general.quantization_version", expMetaQuantizationVersion},
		{"general.file_type", expMetaFileType},
		{"qwen3.context_length", expMetaContextLength},
		{"qwen3.embedding_length", expMetaEmbeddingLength},
		{"qwen3.block_count", expMetaBlockCount},
		{"qwen3.feed_forward_length", expMetaFeedForwardLength},
		{"qwen3.attention.head_count", expMetaHeadCount},
		{"qwen3.attention.head_count_kv", expMetaHeadCountKV},
		{"qwen3.attention.layer_norm_rms_epsilon", expMetaLayerNormRMSEpsilon},
		{"qwen3.attention.key_length", expMetaAttentionKeyLength},
		{"qwen3.attention.value_length", expMetaAttentionValueLength},
		{"qwen3.rope.freq_base", expMetaRopeFreqBase},
		{"tokenizer.ggml.model", expMetaTokenizerModel},
		{"tokenizer.ggml.pre", expMetaTokenizerPre},
		{"tokenizer.ggml.eos_token_id", expMetaTokenizerEOSTokenID},
		{"tokenizer.ggml.padding_token_id", expMetaTokenizerPaddingTokenID},
		{"tokenizer.ggml.bos_token_id", expMetaTokenizerBOSTokenID},
		{"tokenizer.ggml.add_bos_token", expMetaTokenizerAddBOSToken},
	}

	for _, tt := range metaTests {
		val, exists := info.Metadata[tt.key]
		if !exists {
			t.Errorf("Metadata[%q]: key not found", tt.key)
			continue
		}
		if val != tt.want {
			t.Errorf("Metadata[%q]: got %q, want %q", tt.key, val, tt.want)
		}
	}
}

func TestModelInformationQualifiedIDs(t *testing.T) {
	m, err := models.New()
	if err != nil {
		t.Fatalf("Unable to create models api: %v", err)
	}
	wantFingerprint := m.TokenizerFingerprint("Qwen3-8B-Q8_0")
	if wantFingerprint == "" {
		t.Fatal("TokenizerFingerprint returned an empty fingerprint for the bare model ID")
	}

	tests := []struct {
		name    string
		modelID string
	}{
		{name: "model profile", modelID: "Qwen3-8B-Q8_0/AGENT"},
		{name: "provider model", modelID: "Qwen/Qwen3-8B-Q8_0"},
		{name: "provider model profile", modelID: "Qwen/Qwen3-8B-Q8_0/AGENT"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := m.ModelInformation(tt.modelID)
			if err != nil {
				t.Fatalf("ModelInformation(%q) failed: %v", tt.modelID, err)
			}
			if info.ID != tt.modelID {
				t.Errorf("ID: got %q, want %q", info.ID, tt.modelID)
			}
			if info.Desc != expDesc {
				t.Errorf("Desc: got %q, want %q", info.Desc, expDesc)
			}

			file, err := m.FileInformation(tt.modelID)
			if err != nil {
				t.Fatalf("FileInformation(%q) failed: %v", tt.modelID, err)
			}
			if file.ID != tt.modelID {
				t.Errorf("File ID: got %q, want %q", file.ID, tt.modelID)
			}

			if got := m.TokenizerFingerprint(tt.modelID); got != wantFingerprint {
				t.Errorf("TokenizerFingerprint: got %q, want %q", got, wantFingerprint)
			}
		})
	}
}
