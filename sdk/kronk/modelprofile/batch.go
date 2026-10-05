package modelprofile

import "strings"

// BatchInputMixing describes whether a language-model architecture may mix
// token and embedding rows in one llama.cpp batch.
type BatchInputMixing string

const (
	BatchInputMixingUnknown     BatchInputMixing = ""
	BatchInputMixingSupported   BatchInputMixing = "supported"
	BatchInputMixingUnsupported BatchInputMixing = "unsupported"
)

// BatchCapabilities contains model-level batch composition capabilities.
type BatchCapabilities struct {
	InputMixing BatchInputMixing
}

// batchCapabilities tracks validated support and explicit exclusions from the
// bundled llama.cpp release. Unlisted architectures use the isolated path.
func batchCapabilities(architecture string) BatchCapabilities {
	switch strings.ToLower(architecture) {
	case "qwen", "qwen2", "qwen2moe", "qwen2vl", "qwen3", "qwen3moe",
		"qwen3next", "qwen3vl", "qwen3vlmoe", "qwen35", "qwen35moe",
		"qwen4exp", "qwen3tts":
		return BatchCapabilities{InputMixing: BatchInputMixingSupported}

	case "cogvlm", "deepseek4", "graniteswitch", "eagle3", "dflash", "gemma4-assistant":
		return BatchCapabilities{InputMixing: BatchInputMixingUnsupported}

	default:
		return BatchCapabilities{InputMixing: BatchInputMixingUnknown}
	}
}
