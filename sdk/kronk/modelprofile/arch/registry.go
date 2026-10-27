package arch

import "github.com/ardanlabs/kronk/sdk/kronk/modelprofile"

// architectureAdapter normalizes the facts of architectures it claims. More than
// one adapter may apply to a profile; registry order determines their precedence.
type architectureAdapter interface {
	Name() string
	Claims(architecture string) bool
	Apply(metadata Metadata, profile *modelprofile.Profile) error
}

// architectureAdapters is composition wiring only. Architecture-specific
// metadata interpretation belongs to the adapter implementations.
var architectureAdapters = []architectureAdapter{
	EmbeddingPurpose{},
	RerankPurpose{},
	ArchitectureModality{},
	Vision{},
	KnownRecurrent{},
	LFM2{},
	GLM5Next{},
	QwenHybrid{},
	DeepSeek41{},
	SharedKVCompanion{},
	OwnKVCompanion{},
}
