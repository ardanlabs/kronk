package arch

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// Resolve translates raw GGUF metadata into normalized model capabilities.
// Missing optional fields retain their zero values; consumers validate the
// subset required for their operation.
func Resolve(values map[string]string) modelprofile.Profile {
	return ResolveForModel(values, "")
}

// ResolveForModel resolves model facts using metadata first and the model ID as
// a compatibility hint for older GGUFs without explicit purpose metadata.
func ResolveForModel(values map[string]string, modelID string) modelprofile.Profile {
	metadata := NewMetadata(values)
	profile := Generic(metadata)

	for _, adapter := range architectureAdapters {
		if !adapter.Claims(profile.Architecture) {
			continue
		}
		if err := adapter.Apply(metadata, &profile); err != nil {
			modelprofile.AddIssue(&profile, fmt.Errorf("%s profile: %w", adapter.Name(), err))
		}
	}
	resolvePurpose(metadata, modelID, &profile)
	if profile.Role == modelprofile.RoleLanguage {
		profile.Batch = BatchCapabilities(profile.Architecture)
	}
	profile.SupportsTensorParallel = profile.Role == modelprofile.RoleLanguage && SupportsTensorParallel(profile.Architecture)
	profile.Attention.NextNPredictLayers = profile.Speculation.NextNPredictLayers
	profile.SequenceProcessing = SequenceProcessing(metadata, profile)
	return profile
}
