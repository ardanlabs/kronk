package gguf

import (
	"strconv"
	"strings"
)

// ChatTemplate returns the embedded chat template string when present,
// or "" when the model does not carry a chat template.
func ChatTemplate(metadata map[string]string) string {
	return metadata["tokenizer.chat_template"]
}

// HasChatTemplate reports whether the model carries an embedded chat
// template.
func HasChatTemplate(metadata map[string]string) bool {
	return metadata["tokenizer.chat_template"] != ""
}

// GeneralName returns "general.name" or "" when missing.
func GeneralName(metadata map[string]string) string {
	return metadata["general.name"]
}

// GeneralBasename returns "general.basename" or "" when missing.
func GeneralBasename(metadata map[string]string) string {
	return metadata["general.basename"]
}

// GeneralTags returns "general.tags" or "" when missing.
func GeneralTags(metadata map[string]string) string {
	return metadata["general.tags"]
}

// SizeLabel returns "general.size_label" trimmed of surrounding
// whitespace, or "" when missing.
func SizeLabel(metadata map[string]string) string {
	return strings.TrimSpace(metadata["general.size_label"])
}

// ParameterCount returns the model's parameter count from
// "general.parameter_count". Returns 0 when the metadata value is
// missing or unparseable.
func ParameterCount(metadata map[string]string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(metadata["general.parameter_count"]), 10, 64)
	return n
}

// hybridArchPrefixes lists architecture names known to combine
// attention with recurrent (SSM/Mamba/DeltaNet) layers.
var hybridArchPrefixes = []string{
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

// IsHybridArchitecture reports whether the metadata describes a model
// with recurrent layers. Detection looks for SSM/Mamba metadata keys
// and a known-hybrid architecture prefix.
func IsHybridArchitecture(metadata map[string]string) bool {
	arch := strings.ToLower(DetectArchitecture(metadata))
	for _, p := range hybridArchPrefixes {
		if strings.HasPrefix(arch, p) {
			return true
		}
	}

	for k := range metadata {
		lk := strings.ToLower(k)
		if strings.Contains(lk, ".ssm.") || strings.Contains(lk, ".mamba.") || strings.Contains(lk, ".recurrent.") {
			return true
		}
	}

	return false
}
