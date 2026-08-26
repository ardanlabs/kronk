package modelprofile

type architectureAdapter interface {
	Name() string
	Claims(architecture string) bool
	Apply(metadata metadata, profile *Profile) error
}

// architectureAdapters is composition wiring only. Architecture-specific
// metadata interpretation belongs to the adapter implementations.
var architectureAdapters = []architectureAdapter{
	embeddingPurposeAdapter{},
	rerankPurposeAdapter{},
	architectureModalityAdapter{},
	visionAdapter{},
	knownRecurrentAdapter{},
	qwen35Adapter{},
	sharedKVCompanionAdapter{},
}
