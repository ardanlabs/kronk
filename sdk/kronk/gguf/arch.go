package gguf

// DetectArchitecture returns the value of the general.architecture
// metadata key, or "" when it is missing.
func DetectArchitecture(metadata map[string]string) string {
	if arch, ok := metadata["general.architecture"]; ok {
		return arch
	}
	return ""
}
