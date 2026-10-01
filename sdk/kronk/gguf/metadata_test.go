package gguf

import (
	"strings"
	"testing"
)

func TestParseInt64WithFallbackRejectsAmbiguousSuffix(t *testing.T) {
	metadata := map[string]string{
		"language.block_count": "32",
		"vision.block_count":   "24",
	}

	_, err := ParseInt64WithFallback(metadata, "missing.block_count", ".block_count")
	if err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ParseInt64WithFallback: got %v, want ambiguous suffix error", err)
	}
}
