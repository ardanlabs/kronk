package mtp

import (
	"fmt"

	"github.com/ardanlabs/kronk/sdk/kronk/model/internal/speculation"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
)

// ResolveArtifactArchitecture validates the profile's declared MTP contract,
// including recognized but disabled architectures. Qwen35 requires its first
// MTP projection tensor; hasTensor searches shards without loading weights.
func ResolveArtifactArchitecture(metadata map[string]string, hasTensor func(string) (bool, error)) (modelprofile.MTPArchitecture, error) {
	architecture := MetadataArchitecture(metadata)
	if architecture != modelprofile.MTPArchitectureQwen35OwnKV {
		return architecture, nil
	}

	profile := arch.Resolve(metadata)
	blocks := profile.Dimensions.BlockCount
	nextN := profile.Speculation.NextNPredictLayers
	if blocks < nextN {
		return modelprofile.MTPArchitectureNone, fmt.Errorf("MTP layer count %d exceeds block count %d", nextN, blocks)
	}

	// Match llama.cpp's nextn_flags probe. Native loading validates the
	// remaining weights after this probe enables LoadMTP.
	found, err := hasTensor(fmt.Sprintf("blk.%d.nextn.eh_proj.weight", blocks-nextN))
	if err != nil {
		return modelprofile.MTPArchitectureNone, err
	}
	if !found {
		return modelprofile.MTPArchitectureNone, nil
	}
	return architecture, nil
}

// MetadataArchitecture reports a positive MTP layer count for a recognized
// architecture. It does not verify tensor presence or enable the runtime.
func MetadataArchitecture(metadata map[string]string) modelprofile.MTPArchitecture {
	speculation := arch.Resolve(metadata).Speculation
	if speculation.NextNPredictLayers <= 0 {
		return modelprofile.MTPArchitectureNone
	}
	return speculation.MTPArchitecture
}

// MetadataHasAssistant reports a separate-file shared-KV MTP assistant.
func MetadataHasAssistant(metadata map[string]string) bool {
	return arch.Resolve(metadata).Speculation.SharedKVCompanion
}

// ResolveEmbeddedCompatibility checks the hidden-state width consumed by an
// embedded MTP head. Automatic mode falls back on mismatch; explicit MTP fails.
// Qwen4Exp can expose an integral multiple of the target embedding width.
func ResolveEmbeddedCompatibility(plan speculation.Plan, targetEmbeddingWidth, mtpOutputWidth int32) (speculation.Plan, error) {
	if plan.Source != speculation.SourceMTP || plan.MTPArtifact != speculation.MTPArtifactEmbedded {
		return plan, nil
	}
	compatible := targetEmbeddingWidth > 0 && targetEmbeddingWidth == mtpOutputWidth
	if plan.MTPArchitecture == speculation.MTPArchitectureQwen4ExpOwnKV {
		compatible = targetEmbeddingWidth > 0 && mtpOutputWidth >= targetEmbeddingWidth && mtpOutputWidth%targetEmbeddingWidth == 0
	}
	if compatible {
		return plan, nil
	}

	if plan.Mode == speculation.ModeMTP {
		return speculation.Plan{}, fmt.Errorf("embedded MTP output width %d does not match target embedding width %d", mtpOutputWidth, targetEmbeddingWidth)
	}

	return speculation.Plan{Mode: plan.Mode}, nil
}
