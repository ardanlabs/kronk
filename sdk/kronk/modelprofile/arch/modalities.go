package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// ArchitectureModality identifies modalities declared by architecture names.
type ArchitectureModality struct{}

// Name returns the adapter's diagnostic name.
func (ArchitectureModality) Name() string { return "architecture-modalities" }

// Claims reports whether the architecture name declares audio or video support.
func (ArchitectureModality) Claims(architecture string) bool {
	architecture = strings.ToLower(architecture)
	return strings.Contains(architecture, "any-to-any") ||
		strings.Contains(architecture, "omni") ||
		strings.Contains(architecture, "audio") ||
		strings.Contains(architecture, "video")
}

// Apply combines architecture-declared modalities with the generic metadata hints.
func (ArchitectureModality) Apply(_ Metadata, profile *modelprofile.Profile) error {
	architecture := strings.ToLower(profile.Architecture)
	anyToAny := strings.Contains(architecture, "any-to-any") || strings.Contains(architecture, "omni")
	profile.Modalities.Audio = profile.Modalities.Audio || anyToAny || strings.Contains(architecture, "audio")
	profile.Modalities.Video = profile.Modalities.Video || anyToAny || strings.Contains(architecture, "video")
	return nil
}
