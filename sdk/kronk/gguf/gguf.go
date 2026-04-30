// Package gguf provides shared, leaf-level helpers for parsing GGUF
// metadata and computing the KV-cache portion of VRAM. It is intentionally
// dependency-free (no imports of sdk/kronk/model or sdk/tools/models) so
// both sides can call into it without inducing a cycle.
//
// Higher-level orchestration (MoE detection, WeightBreakdown,
// compute-buffer estimates, HuggingFace fetchers, GPU-layer splits)
// remains in sdk/tools/models. SDK-side log diagnostics and the loaded
// Model type stay in sdk/kronk/model.
package gguf

// Bytes per element constants for KV cache types. Mirrors the byte width of
// the corresponding ggml_type values used by llama.cpp.
const (
	BytesPerElementF32  int64 = 4 // 32-bit float
	BytesPerElementF16  int64 = 2 // 16-bit float
	BytesPerElementBF16 int64 = 2 // Brain float 16
	BytesPerElementQ8_0 int64 = 1 // 8-bit quantization
	BytesPerElementQ4_0 int64 = 1 // 4-bit quantization
	BytesPerElementQ4_1 int64 = 1 // 4-bit quantization
	BytesPerElementQ5_0 int64 = 1 // 5-bit quantization
	BytesPerElementQ5_1 int64 = 1 // 5-bit quantization
)

// GGML type IDs (a subset of the ggml_type enum from llama.cpp). These
// values match the integer constants used both by yzma's llama.GGMLType
// and sdk/kronk/model.GGMLType so callers can pass either through int32
// without any further translation table.
const (
	ggmlTypeF16  int32 = 1
	ggmlTypeQ4_0 int32 = 2
	ggmlTypeQ4_1 int32 = 3
	ggmlTypeQ5_0 int32 = 6
	ggmlTypeQ5_1 int32 = 7
	ggmlTypeQ8_0 int32 = 8
	ggmlTypeBF16 int32 = 30
	ggmlTypeF32  int32 = 50
)

// BytesPerElement returns the byte width of the supplied ggml_type ID.
// Unknown / "auto" values fall back to F16 (the llama.cpp default).
func BytesPerElement(typeID int32) int64 {
	switch typeID {
	case ggmlTypeF32:
		return BytesPerElementF32
	case ggmlTypeF16:
		return BytesPerElementF16
	case ggmlTypeBF16:
		return BytesPerElementBF16
	case ggmlTypeQ8_0:
		return BytesPerElementQ8_0
	case ggmlTypeQ4_0:
		return BytesPerElementQ4_0
	case ggmlTypeQ4_1:
		return BytesPerElementQ4_1
	case ggmlTypeQ5_0:
		return BytesPerElementQ5_0
	case ggmlTypeQ5_1:
		return BytesPerElementQ5_1
	default:
		return BytesPerElementF16
	}
}

// MaxBytesPerElement returns the larger of two ggml_type widths. KV cache
// math assumes a single per-element width; when the K and V cache types
// differ we conservatively use the wider of the two so size estimates do
// not under-count.
func MaxBytesPerElement(typeKID, typeVID int32) int64 {
	bytesK := BytesPerElement(typeKID)
	bytesV := BytesPerElement(typeVID)
	if bytesK > bytesV {
		return bytesK
	}
	return bytesV
}
