package modelprofile

import "strings"

type sharedKVCompanionAdapter struct{}

func (sharedKVCompanionAdapter) Name() string { return "shared-kv-mtp-companion" }

func (sharedKVCompanionAdapter) Claims(architecture string) bool {
	return strings.Contains(strings.ToLower(architecture), "assistant")
}

func (sharedKVCompanionAdapter) Apply(_ metadata, profile *Profile) error {
	profile.Speculation.SharedKVCompanion = profile.Speculation.NextNPredictLayers > 0
	if profile.Speculation.SharedKVCompanion {
		profile.Speculation.MTPArchitecture = MTPArchitectureGemmaSharedKV
	}
	return nil
}

type ownKVCompanionAdapter struct{}

func (ownKVCompanionAdapter) Name() string { return "own-kv-mtp-companion" }

func (ownKVCompanionAdapter) Claims(architecture string) bool {
	switch strings.ToLower(architecture) {
	case "qwen35", "qwen35moe", "qwen4exp", "nemotron_h_moe":
		return true
	default:
		return false
	}
}

func (ownKVCompanionAdapter) Apply(_ metadata, profile *Profile) error {
	profile.Speculation.OwnKVCompanion = profile.Speculation.NextNPredictLayers > 0
	if !profile.Speculation.OwnKVCompanion {
		return nil
	}

	switch strings.ToLower(profile.Architecture) {
	case "qwen35", "qwen35moe":
		profile.Speculation.MTPArchitecture = MTPArchitectureQwen35OwnKV
	case "qwen4exp":
		profile.Speculation.MTPArchitecture = MTPArchitectureQwen4ExpOwnKV
	case "nemotron_h_moe":
		profile.Speculation.MTPArchitecture = MTPArchitectureNemotronOwnKV
	}
	return nil
}
