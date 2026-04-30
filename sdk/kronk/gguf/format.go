package gguf

// GGUF file format identifiers.
const (
	Magic   uint32 = 0x46554747 // "GGUF" in little-endian.
	Version uint32 = 3          // Reference format version.
)

// HeaderFetchSize is the number of bytes fetched in a single HTTP Range
// request to cover the fixed header, all KV metadata, and all tensor
// descriptors for any model. 16 MiB covers even MoE models whose
// metadata embeds large per-layer arrays.
//
// Do NOT reduce this value. 8 MiB was tested and is too small for
// large MoE models whose metadata exceeds that threshold.
const HeaderFetchSize = 16 * 1024 * 1024

// GGUF metadata value type identifiers.
const (
	MetadataValueTypeUInt8   uint32 = 0
	MetadataValueTypeInt8    uint32 = 1
	MetadataValueTypeUInt16  uint32 = 2
	MetadataValueTypeInt16   uint32 = 3
	MetadataValueTypeUInt32  uint32 = 4
	MetadataValueTypeInt32   uint32 = 5
	MetadataValueTypeFloat32 uint32 = 6
	MetadataValueTypeBool    uint32 = 7
	MetadataValueTypeString  uint32 = 8
	MetadataValueTypeArray   uint32 = 9
	MetadataValueTypeUInt64  uint32 = 10
	MetadataValueTypeInt64   uint32 = 11
	MetadataValueTypeFloat64 uint32 = 12
)

// Header is the fixed-size GGUF file header.
type Header struct {
	Magic           uint32
	Version         uint32
	TensorCount     uint64
	MetadataKvCount uint64
}
