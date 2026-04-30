package gguf

// DetectArchitecture returns the value of the general.architecture
// metadata key, or "" when it is missing.
func DetectArchitecture(metadata map[string]string) string {
	if arch, ok := metadata["general.architecture"]; ok {
		return arch
	}
	return ""
}

// IsVisionEncoder reports whether the named architecture is a vision
// encoder (CLIP-style projector), which sizes its KV cache differently
// (effectively zero) from a transformer language model.
func IsVisionEncoder(arch string) bool {
	switch arch {
	case "clip", "qwen2vl":
		return true
	}
	return false
}
