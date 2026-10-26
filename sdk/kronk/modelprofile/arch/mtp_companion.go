package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// SharedKVCompanion normalizes assistant artifacts sharing the target's KV cache.
type SharedKVCompanion struct{}

// Name returns the adapter's diagnostic name.
func (SharedKVCompanion) Name() string { return "shared-kv-mtp-companion" }

// Claims reports whether the architecture is an assistant artifact.
func (SharedKVCompanion) Claims(architecture string) bool {
	return strings.Contains(strings.ToLower(architecture), "assistant")
}

// Apply identifies an assistant with a declared shared-KV MTP head.
func (SharedKVCompanion) Apply(_ Metadata, profile *modelprofile.Profile) error {
	profile.Speculation.SharedKVCompanion = profile.Speculation.NextNPredictLayers > 0
	if profile.Speculation.SharedKVCompanion {
		profile.Speculation.MTPArchitecture = modelprofile.MTPArchitectureGemmaSharedKV
	}
	return nil
}

// OwnKVCompanion normalizes MTP artifacts with an independent KV cache.
type OwnKVCompanion struct{}

// Name returns the adapter's diagnostic name.
func (OwnKVCompanion) Name() string { return "own-kv-mtp-companion" }

// Claims reports whether the architecture supports an own-KV companion.
func (OwnKVCompanion) Claims(architecture string) bool {
	switch strings.ToLower(architecture) {
	case "qwen35", "qwen35moe", "qwen4exp", "nemotron_h_moe":
		return true
	default:
		return false
	}
}

// Apply identifies the own-KV runtime contract for a declared MTP head.
func (OwnKVCompanion) Apply(_ Metadata, profile *modelprofile.Profile) error {
	profile.Speculation.OwnKVCompanion = profile.Speculation.NextNPredictLayers > 0
	if !profile.Speculation.OwnKVCompanion {
		return nil
	}

	switch strings.ToLower(profile.Architecture) {
	case "qwen35", "qwen35moe":
		profile.Speculation.MTPArchitecture = modelprofile.MTPArchitectureQwen35OwnKV
	case "qwen4exp":
		profile.Speculation.MTPArchitecture = modelprofile.MTPArchitectureQwen4ExpOwnKV
	case "nemotron_h_moe":
		profile.Speculation.MTPArchitecture = modelprofile.MTPArchitectureNemotronOwnKV
	}
	return nil
}
