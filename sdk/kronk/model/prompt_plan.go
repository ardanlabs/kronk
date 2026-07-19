package model

import (
	"crypto/sha256"
	"errors"
	"slices"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/hybridgroup/yzma/pkg/mtmd"
)

type promptUnit struct {
	token   llama.Token
	media   [sha256.Size]byte
	isMedia bool
}

type promptPlan struct {
	units      []promptUnit
	textTokens int
	mediaCount int
}

func buildPromptPlan(vocab llama.Vocab, prompt string, media [][]byte) (promptPlan, error) {
	// A token-v2 media plan is used as an immutable cache identity. An
	// automatically appended EOS is not safe at an append boundary: the EOS
	// belongs at the end of the old render, but must move when more text is
	// appended. Fall back to the authoritative mtmd rebuild path instead of
	// treating that old EOS as an immutable prefix token.
	if llama.VocabGetAddEOS(vocab) {
		return promptPlan{}, errors.New("prompt-plan: automatic EOS is not append-safe")
	}

	return buildPromptPlanTokens(prompt, mtmd.DefaultMarker(), media, llama.VocabBOS(vocab), llama.VocabGetAddBOS(vocab), func(text string) []llama.Token {
		// mtmd tokenizes each marker-delimited text chunk independently.
		// Special tokens are added globally below, never once per chunk.
		return llama.Tokenize(vocab, text, false, true)
	})
}

func buildPromptPlanTokens(prompt, marker string, media [][]byte, bos llama.Token, addBOS bool, tokenize func(string) []llama.Token) (promptPlan, error) {
	segments := strings.Split(prompt, marker)
	if len(segments)-1 != len(media) {
		return promptPlan{}, errors.New("prompt-plan: marker/media count mismatch")
	}

	var plan promptPlan
	if addBOS {
		plan.units = append(plan.units, promptUnit{token: bos})
		plan.textTokens++
	}
	for i, segment := range segments {
		tokens := tokenize(segment)
		for _, token := range tokens {
			plan.units = append(plan.units, promptUnit{token: token})
		}
		plan.textTokens += len(tokens)
		if i < len(media) {
			plan.units = append(plan.units, promptUnit{media: sha256.Sum256(media[i]), isMedia: true})
			plan.mediaCount++
		}
	}

	return plan, nil
}

func (p promptPlan) hasPrefix(prefix promptPlan) bool {
	return len(prefix.units) <= len(p.units) && slices.Equal(p.units[:len(prefix.units)], prefix.units)
}

func (p promptPlan) equal(other promptPlan) bool {
	return slices.Equal(p.units, other.units)
}

func (p promptPlan) textTail(prefix promptPlan) ([]llama.Token, bool) {
	if !p.hasPrefix(prefix) {
		return nil, false
	}
	tokens := make([]llama.Token, 0, len(p.units)-len(prefix.units))
	for _, unit := range p.units[len(prefix.units):] {
		if unit.isMedia {
			return nil, false
		}
		tokens = append(tokens, unit.token)
	}
	return tokens, true
}
