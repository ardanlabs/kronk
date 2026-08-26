package modelprofile

import "strings"

type visionAdapter struct{}

func (visionAdapter) Name() string { return "vision-encoder" }

func (visionAdapter) Claims(architecture string) bool {
	switch strings.ToLower(architecture) {
	case "clip", "qwen2vl":
		return true
	default:
		return false
	}
}

func (visionAdapter) Apply(_ metadata, profile *Profile) error {
	profile.Role = RoleVisionEncoder
	return nil
}
