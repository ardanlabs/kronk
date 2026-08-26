package modelprofile

import "strings"

type embeddingPurposeAdapter struct{}

func (embeddingPurposeAdapter) Name() string { return "embedding-purpose" }

func (embeddingPurposeAdapter) Claims(architecture string) bool {
	return strings.Contains(strings.ToLower(architecture), "embed")
}

func (embeddingPurposeAdapter) Apply(_ metadata, profile *Profile) error {
	profile.Purpose = PurposeEmbedding
	return nil
}

type rerankPurposeAdapter struct{}

func (rerankPurposeAdapter) Name() string { return "rerank-purpose" }

func (rerankPurposeAdapter) Claims(architecture string) bool {
	architecture = strings.ToLower(architecture)
	return strings.Contains(architecture, "rerank") || strings.Contains(architecture, "bert")
}

func (rerankPurposeAdapter) Apply(_ metadata, profile *Profile) error {
	if profile.Purpose != PurposeEmbedding {
		profile.Purpose = PurposeRerank
	}
	return nil
}
