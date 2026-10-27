package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// Vision identifies projection artifacts rather than language models.
type Vision struct{}

// Name returns the adapter's diagnostic name.
func (Vision) Name() string { return "vision-encoder" }

// Claims reports whether the architecture is a vision encoder.
func (Vision) Claims(architecture string) bool {
	switch strings.ToLower(architecture) {
	case "clip", "qwen2vl":
		return true
	default:
		return false
	}
}

// Apply marks the artifact as a vision encoder.
func (Vision) Apply(_ Metadata, profile *modelprofile.Profile) error {
	profile.Role = modelprofile.RoleVisionEncoder
	return nil
}
