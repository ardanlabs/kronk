package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// KnownRecurrent identifies architectures with recurrent sequence state.
type KnownRecurrent struct{}

// Name returns the adapter's diagnostic name.
func (KnownRecurrent) Name() string { return "known-recurrent" }

// Claims reports whether the architecture uses recurrent sequence state.
func (KnownRecurrent) Claims(architecture string) bool {
	architecture = strings.ToLower(architecture)
	for _, prefix := range recurrentArchitecturePrefixes {
		if strings.HasPrefix(architecture, prefix) {
			return true
		}
	}
	return false
}

// Apply marks the profile as requiring recurrent sequence state.
func (KnownRecurrent) Apply(_ Metadata, profile *modelprofile.Profile) error {
	profile.MemorySemantics = modelprofile.MemoryRecurrent
	return nil
}

var recurrentArchitecturePrefixes = []string{
	"lfm2",
	"jamba",
	"mamba",
	"recurrentgemma",
	"qwen3next",
	"qwen3-next",
	"granite-hybrid",
	"granitemoehybrid",
	"nemotron-h",
	"nemotronh",
	"nemotron_h",
}
