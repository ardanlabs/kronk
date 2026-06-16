package models

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

// ModelInfo represents the model's card information.
type ModelInfo struct {
	ID            string
	HasProjection bool
	HasMTP        bool
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

	return ModelInfoFromPath(modelID, path.ModelFiles, path.ProjFile, path.MTPFile)
}

// ModelInfoFromPath builds ModelInfo directly from on-disk model file paths,
// without requiring a catalog/basePath. It backs ModelInformation and lets
// callers that already hold raw file paths (e.g. SDK auto-tune via kronk.New)
// run the analysis. id is used only for the gpt/embed/rerank name heuristics
// and the ID field; when empty it is derived from the first model file name.
func ModelInfoFromPath(id string, modelFiles []string, projFile string, mtpFile string) (ModelInfo, error) {
	if len(modelFiles) == 0 {
		return ModelInfo{}, fmt.Errorf("model-info-from-path: no model files provided")
	}

	if id == "" {
		base := filepath.Base(modelFiles[0])
		id = strings.TrimSuffix(base, filepath.Ext(base))
	}

	var totalSize uint64
	for _, mf := range modelFiles {
		info, err := os.Stat(mf)
		if err != nil {
			return ModelInfo{}, fmt.Errorf("failed to stat file: %w", err)
		}
		totalSize += uint64(info.Size())
	}

	data, err := gguf.ReadHeaderBytes(modelFiles[0])
	if err != nil {
		return ModelInfo{}, err
	}

	metadata, err := gguf.ParseMetadata(data)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("model-information: %w", err)
	}

	lowerID := strings.ToLower(id)

	mi := ModelInfo{
		ID:            id,
		HasProjection: projFile != "",
		HasMTP:        mtpFile != "",
		Desc:          metadata["general.name"],
		Size:          totalSize,
		IsGPTModel:    strings.Contains(lowerID, "gpt"),
		IsEmbedModel:  strings.Contains(lowerID, "embed"),
		IsRerankModel: strings.Contains(lowerID, "rerank"),
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
