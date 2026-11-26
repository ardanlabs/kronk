// Package dspark provides DSpark compatibility and speculative-decoding policy.
package dspark

import (
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
)

// ProposalLimit validates the qualified Inkling2 Markov-DFlash metadata contract
// and caps a positive override at the trained proposal limit. Native tensor and
// vocabulary validation remains with the resource loader.
func ProposalLimit(target, draft map[string]string, requested int) (int, error) {
	if target["general.architecture"] != "inkling2_loop" || draft["general.architecture"] != "dflash" {
		return 0, fmt.Errorf("unsupported target/DSpark architectures %q/%q; only Inkling2 loop with a Markov DFlash companion is qualified", target["general.architecture"], draft["general.architecture"])
	}
	for key, want := range map[string]string{
		"dflash.sample_from_anchor":  "false",
		"dflash.has_markov_context":  "true",
		"dflash.has_confidence_head": "false",
	} {
		if draft[key] != want {
			return 0, fmt.Errorf("unsupported %s=%q, require %s", key, draft[key], want)
		}
	}
	if causal := draft["dflash.attention.causal"]; causal != "" && causal != "false" {
		return 0, fmt.Errorf("DSpark requires noncausal attention, got %q", causal)
	}
	profile := arch.Resolve(target)
	if err := profile.ValidateForAnalysis(); err != nil {
		return 0, fmt.Errorf("DSpark target profile: %w", err)
	}
	width := profile.Dimensions.EmbeddingLength
	if width <= 0 {
		return 0, fmt.Errorf("invalid target embedding width %q", target["inkling2_loop.embedding_length"])
	}
	draftWidth, err := gguf.ParseInt64(draft, "dflash.embedding_length")
	if err != nil || draftWidth != width {
		return 0, fmt.Errorf("DSpark embedding width %q does not match target width %d", draft["dflash.embedding_length"], width)
	}
	// Loop models reuse physical blocks. Extraction IDs refer to expanded
	// logical layers, already normalized by the architecture profile.
	layers := profile.Attention.RecurrentLayers
	extract, err := gguf.ParseInt64OrArray(draft, "dflash.target_layers")
	if err != nil {
		return 0, fmt.Errorf("DSpark target layers: %w", err)
	}
	seen := make(map[int64]bool, len(extract))
	for _, layer := range extract {
		// Inkling2 exposes its normalized terminal output at layer == n_layer.
		if layer < 0 || layer > layers || seen[layer] {
			return 0, fmt.Errorf("invalid or duplicate DSpark target layer %d for %d-layer target", layer, layers)
		}
		seen[layer] = true
	}
	if target["tokenizer.ggml.model"] == "" || target["tokenizer.ggml.tokens"] == "" {
		return 0, fmt.Errorf("target tokenizer metadata is missing")
	}
	mask, err := gguf.ParseInt64(draft, "tokenizer.ggml.mask_token_id")
	if err != nil || mask < 0 || mask > 1<<31-1 {
		return 0, fmt.Errorf("invalid DSpark mask token ID %q", draft["tokenizer.ggml.mask_token_id"])
	}
	for _, values := range []map[string]string{target, draft} {
		for key := range values {
			// The sidecar designates a mask token that the target need not
			// designate. This does not change the underlying token-ID mapping.
			if key == "tokenizer.ggml.mask_token_id" {
				continue
			}
			if strings.HasPrefix(key, "tokenizer.ggml.") && target[key] != draft[key] {
				return 0, fmt.Errorf("DSpark tokenizer metadata %q differs from target", key)
			}
		}
	}
	block, err := gguf.ParseInt64(draft, "dflash.block_size")
	if err != nil || block < 2 || block > 1<<31-1 {
		return 0, fmt.Errorf("invalid DSpark trained block size %q", draft["dflash.block_size"])
	}
	nDraft := int(block - 1)
	if requested > 0 {
		nDraft = min(nDraft, requested)
	}
	return nDraft, nil
}
