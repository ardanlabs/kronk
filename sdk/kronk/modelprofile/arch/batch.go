package arch

import (
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

// BatchCapabilities tracks validated support and explicit exclusions from the
// bundled llama.cpp release. Unlisted architectures use the isolated path.
func BatchCapabilities(architecture string) modelprofile.BatchCapabilities {
	switch strings.ToLower(architecture) {
	case "qwen", "qwen2", "qwen2moe", "qwen2vl", "qwen3", "qwen3moe",
		"qwen3next", "qwen3vl", "qwen3vlmoe", "qwen35", "qwen35moe",
		"qwen4exp", "qwen3tts":
		return modelprofile.BatchCapabilities{InputMixing: modelprofile.BatchInputMixingSupported}

	case "cogvlm", "deepseek4", "graniteswitch", "eagle3", "dflash", "gemma4-assistant":
		return modelprofile.BatchCapabilities{InputMixing: modelprofile.BatchInputMixingUnsupported}

	default:
		return modelprofile.BatchCapabilities{InputMixing: modelprofile.BatchInputMixingUnknown}
	}
}
