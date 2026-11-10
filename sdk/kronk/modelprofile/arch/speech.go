package arch

import (
	"slices"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// ResolveWithTensors supplements metadata resolution with allocation families
// that require tensor descriptors. Speech purpose alone does not establish a
// supported speech graph, and Chatterbox's decoder metadata also describes its
// multilingual variant. Names and paths are never used to select these policies.
func ResolveWithTensors(values map[string]string, tensors []gguf.TensorInfo) modelprofile.Profile {
	profile := Resolve(values)
	var tokenTable, positions, meanflow, multilingual bool
	for _, tensor := range tensors {
		switch tensor.Name {
		case "token_embd.weight":
			tokenTable = slices.Equal(tensor.Dims, []int64{1024, 56839})
		case "position_embd.weight":
			positions = slices.Equal(tensor.Dims, []int64{1024, 8196})
		case "a.gen.est.time_embed_mixer.weight":
			meanflow = slices.Equal(tensor.Dims, []int64{2048, 1024})
		case "a.gen.t3.text_pos_emb":
			multilingual = true
		}
	}
	switch {
	case (profile.Purpose == modelprofile.PurposeSpeech || profile.Purpose == modelprofile.PurposeGeneration) &&
		!profile.HasChatTemplate && profile.Architecture == "gpt2" &&
		profile.Dimensions.BlockCount == 24 && profile.Dimensions.EmbeddingLength == 1024 &&
		profile.Dimensions.HeadCount == 16 && profile.Dimensions.FeedForwardLength == 4096 &&
		values["tokenizer.ggml.bos_token_id"] == "56837" && values["tokenizer.ggml.eos_token_id"] == "56838" &&
		tokenTable && positions:
		profile.ComputeKind = modelprofile.ComputeChatterboxTurboBackbone
	case profile.Architecture == "clip" && values["clip.has_gen_audio_encoder"] == "true" &&
		values["clip.gen.audio.projector_type"] == "chatterbox" && meanflow && !multilingual:
		profile.ComputeKind = modelprofile.ComputeChatterboxTurboDecoder
	}
	return profile
}
