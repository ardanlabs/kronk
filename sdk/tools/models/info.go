package models

import (
	"fmt"
	"os"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// ModelInfo represents the model's card information.
type ModelInfo struct {
	ID            string
	HasProjection bool
	Desc          string
	Size          uint64
	IsGPTModel    bool
	IsEmbedModel  bool
	IsRerankModel bool
	Metadata      map[string]string
}

// =============================================================================

// ModelInformation reads a GGUF model file and extracts model information.
// The first gguf.HeaderFetchSize bytes are read into memory and parsed via
// the shared sdk/kronk/gguf reader so the SDK and tools side stay in sync
// on metadata semantics (notably ARRAY-typed values).
func (m *Models) ModelInformation(modelID string) (ModelInfo, error) {
	modelID, _, _ = strings.Cut(modelID, "/")

	path, err := m.FullPath(modelID)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("failed to retrieve path modelID[%s] file: %w", modelID, err)
	}

	var totalSize uint64
	for _, mf := range path.ModelFiles {
		info, err := os.Stat(mf)
		if err != nil {
			return ModelInfo{}, fmt.Errorf("failed to stat file: %w", err)
		}
		totalSize += uint64(info.Size())
	}

	data, err := gguf.ReadHeaderBytes(path.ModelFiles[0])
	if err != nil {
		return ModelInfo{}, err
	}

	metadata, err := gguf.ParseMetadata(data)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("model-information: %w", err)
	}

	var isGPTModel bool
	if strings.Contains(modelID, "gpt") {
		isGPTModel = true
	}

	var isEmbedModel bool
	if strings.Contains(modelID, "embed") {
		isEmbedModel = true
	}

	var isRerankModel bool
	if strings.Contains(modelID, "rerank") {
		isRerankModel = true
	}

	mi := ModelInfo{
		ID:            modelID,
		HasProjection: path.ProjFile != "",
		Desc:          metadata["general.name"],
		Size:          totalSize,
		IsGPTModel:    isGPTModel,
		IsEmbedModel:  isEmbedModel,
		IsRerankModel: isRerankModel,
		Metadata:      metadata,
	}

	return mi, nil
}

// TokenizerFingerprint reads a model's GGUF file and extracts a fingerprint
// string that identifies the tokenizer. Models sharing the same fingerprint
// use compatible tokenizers and can be paired for speculative decoding.
// The fingerprint format is "<tokenizer_model>:<tokenizer_pre>".
func (m *Models) TokenizerFingerprint(modelID string) string {
	modelID, _, _ = strings.Cut(modelID, "/")

	path, err := m.FullPath(modelID)
	if err != nil || len(path.ModelFiles) == 0 {
		return ""
	}

	return tokenizerFingerprintFromFile(path.ModelFiles[0])
}

// tokenizerFingerprintFromFile reads a GGUF file and extracts a fingerprint
// string that identifies the tokenizer. The fingerprint format is
// "<tokenizer_model>:<tokenizer_pre>".
func tokenizerFingerprintFromFile(filePath string) string {
	data, err := gguf.ReadHeaderBytes(filePath)
	if err != nil {
		return ""
	}

	metadata, err := gguf.ParseMetadata(data)
	if err != nil {
		return ""
	}

	tokenizerModel := metadata["tokenizer.ggml.model"]
	tokenizerPre := metadata["tokenizer.ggml.pre"]

	if tokenizerModel == "" {
		return ""
	}

	return tokenizerModel + ":" + tokenizerPre
}
