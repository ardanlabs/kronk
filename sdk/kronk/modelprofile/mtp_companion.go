package modelprofile

import "strings"

type sharedKVCompanionAdapter struct{}

func (sharedKVCompanionAdapter) Name() string { return "shared-kv-mtp-companion" }

func (sharedKVCompanionAdapter) Claims(architecture string) bool {
	return strings.Contains(strings.ToLower(architecture), "assistant")
}

func (sharedKVCompanionAdapter) Apply(_ metadata, profile *Profile) error {
	profile.Speculation.SharedKVCompanion = profile.Speculation.NextNPredictLayers > 0
	return nil
}
