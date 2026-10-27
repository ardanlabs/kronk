package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// EmbeddingPurpose identifies embedding architectures.
type EmbeddingPurpose struct{}

// Name returns the adapter's diagnostic name.
func (EmbeddingPurpose) Name() string { return "embedding-purpose" }

// Claims reports whether the architecture name declares embedding support.
func (EmbeddingPurpose) Claims(architecture string) bool {
	return strings.Contains(strings.ToLower(architecture), "embed")
}

// Apply marks the profile as an embedding model.
func (EmbeddingPurpose) Apply(_ Metadata, profile *modelprofile.Profile) error {
	profile.Purpose = modelprofile.PurposeEmbedding
	return nil
}

// RerankPurpose identifies reranking architectures without overriding embedding.
type RerankPurpose struct{}

// Name returns the adapter's diagnostic name.
func (RerankPurpose) Name() string { return "rerank-purpose" }

// Claims reports whether the architecture name declares a reranking family.
func (RerankPurpose) Claims(architecture string) bool {
	architecture = strings.ToLower(architecture)
	return strings.Contains(architecture, "rerank") || strings.Contains(architecture, "bert")
}

// Apply marks the profile as a reranker unless embedding support takes precedence.
func (RerankPurpose) Apply(_ Metadata, profile *modelprofile.Profile) error {
	if profile.Purpose != modelprofile.PurposeEmbedding {
		profile.Purpose = modelprofile.PurposeRerank
	}
	return nil
}
