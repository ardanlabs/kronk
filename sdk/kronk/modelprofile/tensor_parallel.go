package modelprofile

import "strings"

// supportsTensorParallel mirrors llm_arch_supports_sm_tensor in the bundled
// llama.cpp release. Keep this list synchronized when the native dependency is
// updated.
func supportsTensorParallel(architecture string) bool {
	if architecture == "" {
		return false
	}

	switch strings.ToLower(architecture) {
	case "grok", "mpt", "plamo2", "minicpm3", "gemma3n",
		"mamba", "mamba2", "jamba", "falcon-h1", "olmo2", "olmoe",
		"deepseek2", "deepseek32", "hy_v4", "dots3note", "glm-dsa",
		"bitnet", "t5", "nemotron_h", "nemotron_h_moe", "granitehybrid",
		"minimax-01", "minimax-m2", "minimax-m3", "mistral4", "kimi-linear",
		"bailingmoe3", "kimi-k3", "qwen3tts", "qwen4exp":
		return false
	default:
		return true
	}
}
