package arch

import (
	"strings"
	"unicode"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// resolvePurpose is shared by runtime, catalog discovery, and offline analysis.
// Explicit metadata wins over names; names win over architecture defaults.
func resolvePurpose(metadata Metadata, modelID string, profile *modelprofile.Profile) {
	if profile.Role != modelprofile.RoleLanguage {
		return
	}
	if profile.Decision.Type != "" {
		return
	}

	pooling := metadata.value(profile.Architecture + ".pooling_type")
	switch pooling {
	case "1", "2", "3":
		profile.Purpose = modelprofile.PurposeEmbedding
		return
	case "4":
		profile.Purpose = modelprofile.PurposeRerank
		return
	}

	hint := strings.ToLower(strings.Join([]string{
		modelID, metadata.value("general.name"), metadata.value("general.basename"),
		metadata.value("general.tags"), profile.Architecture,
	}, " "))
	if protocol := decisionNameHint(hint); protocol != "" {
		profile.Decision.Type = protocol
		return
	}
	switch {
	case strings.Contains(hint, "rerank"):
		profile.Purpose = modelprofile.PurposeRerank
	case strings.Contains(hint, "embed"):
		profile.Purpose = modelprofile.PurposeEmbedding
	case strings.Contains(profile.Architecture, "bert"):
		profile.Purpose = modelprofile.PurposeEmbedding
	}
}

func decisionNameHint(name string) string {
	switch {
	case strings.Contains(name, "openjev"):
		return "openjev"
	case strings.Contains(name, "jev-style"):
		return "jev-style"
	case strings.Contains(name, "julia-1"):
		return "laya"
	}
	for _, token := range strings.FieldsFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		switch token {
		case "laya", "lev", "kev":
			return token
		}
	}
	return ""
}
