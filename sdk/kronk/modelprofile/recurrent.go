package modelprofile

import "strings"

type knownRecurrentAdapter struct{}

func (knownRecurrentAdapter) Name() string { return "known-recurrent" }

func (knownRecurrentAdapter) Claims(architecture string) bool {
	architecture = strings.ToLower(architecture)
	for _, prefix := range recurrentArchitecturePrefixes {
		if strings.HasPrefix(architecture, prefix) {
			return true
		}
	}
	return false
}

func (knownRecurrentAdapter) Apply(_ metadata, profile *Profile) error {
	profile.MemorySemantics = MemoryRecurrent
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
}
