package gguf

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseFloat64 returns the named metadata value parsed as a 64-bit
// float. Missing keys yield an error so callers can distinguish absent
// metadata from explicitly-zero values.
func ParseFloat64(metadata map[string]string, key string) (float64, error) {
	val, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("parse-metadata-float64: metadata key %q not found", key)
	}
	return strconv.ParseFloat(val, 64)
}

// ParseInt64 returns the named metadata value parsed as a signed 64-bit
// integer. Missing keys yield an error so callers can distinguish absent
// metadata from explicitly-zero values.
func ParseInt64(metadata map[string]string, key string) (int64, error) {
	val, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("parse-metadata-int64: metadata key %q not found", key)
	}
	return strconv.ParseInt(val, 10, 64)
}

// ParseInt64WithFallback returns the named metadata value parsed as a
// signed 64-bit integer. When the primary key is missing the map is
// scanned for any key matching the supplied suffix, supporting renamed /
// arch-prefixed metadata across different GGUF producers.
func ParseInt64WithFallback(metadata map[string]string, key string, suffix string) (int64, error) {
	val, ok := metadata[key]
	if ok {
		return strconv.ParseInt(val, 10, 64)
	}

	for k, v := range metadata {
		if strings.HasSuffix(k, suffix) {
			return strconv.ParseInt(v, 10, 64)
		}
	}

	return 0, fmt.Errorf("parse-metadata-int64: metadata key %q not found", key)
}

// ParseInt64OrArrayAvg parses a metadata value that may be either a
// single integer (e.g. "8") or a per-layer array. Arrays produced by our
// own GGUF parser are space-separated ("[0 0 8 ...]") while arrays
// surfaced by llama.cpp's gguf_kv_to_str are comma-separated
// ("[0, 0, 8, ...]"); both are accepted. For arrays, the average of all
// elements is returned. This handles hybrid architectures like LFM2 and
// MoE Gemma where head_count_kv varies per layer.
func ParseInt64OrArrayAvg(metadata map[string]string, key string) (int64, error) {
	val, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("parse-metadata-int64: metadata key %q not found", key)
	}

	// Try scalar first.
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		return n, nil
	}

	// Try array format: "[v1 v2 v3 ...]" or "[v1, v2, v3, ...]".
	trimmed := strings.TrimSpace(val)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return 0, fmt.Errorf("parse-metadata-int64: unable to parse %q for key %q", val, key)
	}

	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return 0, fmt.Errorf("parse-metadata-int64: empty array for key %q", key)
	}

	splitFn := func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n'
	}
	fields := strings.FieldsFunc(inner, splitFn)

	var sum int64
	var count int64
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		n, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse-metadata-int64: unable to parse array element %q for key %q: %w", f, key, err)
		}
		sum += n
		count++
	}

	if count == 0 {
		return 0, fmt.Errorf("parse-metadata-int64: empty array for key %q", key)
	}

	return sum / count, nil
}

// ResolveKVLengths returns key_length and value_length for VRAM
// calculation. It first checks for explicit metadata keys. When those are
// missing (e.g. audio models like Qwen2-Audio or LFM2 hybrids), it falls
// back to embedding_length / head_count which is the same default
// llama.cpp uses internally.
func ResolveKVLengths(metadata map[string]string, arch string) (keyLen int64, valLen int64, err error) {
	keyLen, keyErr := ParseInt64(metadata, arch+".attention.key_length")
	valLen, valErr := ParseInt64(metadata, arch+".attention.value_length")

	if keyErr == nil && valErr == nil {
		return keyLen, valLen, nil
	}

	embLen, err := ParseInt64(metadata, arch+".embedding_length")
	if err != nil {
		return 0, 0, fmt.Errorf("resolve-kv-lengths: key_length and embedding_length both missing")
	}

	headCount, err := ParseInt64(metadata, arch+".attention.head_count")
	if err != nil || headCount == 0 {
		return 0, 0, fmt.Errorf("resolve-kv-lengths: key_length and head_count both missing")
	}

	derived := embLen / headCount

	if keyErr != nil {
		keyLen = derived
	}
	if valErr != nil {
		valLen = derived
	}

	return keyLen, valLen, nil
}
