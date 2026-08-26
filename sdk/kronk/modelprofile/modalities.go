package modelprofile

import "strings"

type architectureModalityAdapter struct{}

func (architectureModalityAdapter) Name() string { return "architecture-modalities" }

func (architectureModalityAdapter) Claims(architecture string) bool {
	architecture = strings.ToLower(architecture)
	return strings.Contains(architecture, "any-to-any") ||
		strings.Contains(architecture, "omni") ||
		strings.Contains(architecture, "audio") ||
		strings.Contains(architecture, "video")
}

func (architectureModalityAdapter) Apply(_ metadata, profile *Profile) error {
	architecture := strings.ToLower(profile.Architecture)
	anyToAny := strings.Contains(architecture, "any-to-any") || strings.Contains(architecture, "omni")
	profile.Modalities.Audio = profile.Modalities.Audio || anyToAny || strings.Contains(architecture, "audio")
	profile.Modalities.Video = profile.Modalities.Video || anyToAny || strings.Contains(architecture, "video")
	return nil
}
