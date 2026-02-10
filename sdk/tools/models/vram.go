package models

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Context window size constants (in tokens).
const (
	ContextWindow1K   int64 = 1024
	ContextWindow2K   int64 = 2048
	ContextWindow4K   int64 = 4096
	ContextWindow8K   int64 = 8192
	ContextWindow16K  int64 = 16384
	ContextWindow32K  int64 = 32768
	ContextWindow64K  int64 = 65536
	ContextWindow128K int64 = 131072
	ContextWindow256K int64 = 262144
)

// Bytes per element constants for cache types.
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

// Slot count constants.
const (
	Slots1 int64 = 1
	Slots2 int64 = 2
	Slots3 int64 = 3
	Slots4 int64 = 4
	Slots5 int64 = 5
)

// VRAMConfig contains the user-provided parameters for VRAM calculation
// that cannot be extracted from the model file.
type VRAMConfig struct {
	ContextWindow   int64 // n_ctx - context window size (e.g., 8192, 131072)
	BytesPerElement int64 // Depends on cache type: q8_0=1, f16=2
	Slots           int64 // n_seq_max - number of concurrent sequences
}

// VRAM contains the calculated VRAM requirements.
type VRAM struct {
	Input              VRAMInput // Input parameters used for calculation
	KVPerTokenPerLayer int64     // Bytes per token per layer
	KVPerSlot          int64     // Bytes per slot
	SlotMemory         int64     // Total KV cache memory in bytes
	TotalVRAM          int64     // Model size + slot memory in bytes
}

// CalculateVRAM retrieves model metadata and computes the VRAM requirements.
func (m *Models) CalculateVRAM(modelID string, cfg VRAMConfig) (VRAM, error) {
	info, err := m.ModelInformation(modelID)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: failed to retrieve model info: %w", err)
	}

	arch := detectArchitecture(info.Metadata)
	if arch == "" {
		return VRAM{}, fmt.Errorf("calculate-vram: unable to detect model architecture")
	}

	if isVisionEncoder(arch) {
		return VRAM{
			Input:     VRAMInput{ModelSizeBytes: int64(info.Size)},
			TotalVRAM: int64(info.Size),
		}, nil
	}

	blockCount, err := parseMetadataInt64WithFallback(info.Metadata, arch+".block_count", ".block_count")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: failed to parse block_count: %w", err)
	}

	headCountKV, err := parseMetadataInt64(info.Metadata, arch+".attention.head_count_kv")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: failed to parse head_count_kv: %w", err)
	}

	keyLength, err := parseMetadataInt64(info.Metadata, arch+".attention.key_length")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: failed to parse key_length: %w", err)
	}

	valueLength, err := parseMetadataInt64(info.Metadata, arch+".attention.value_length")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: failed to parse value_length: %w", err)
	}

	input := VRAMInput{
		ModelSizeBytes:  int64(info.Size),
		ContextWindow:   cfg.ContextWindow,
		BlockCount:      blockCount,
		HeadCountKV:     headCountKV,
		KeyLength:       keyLength,
		ValueLength:     valueLength,
		BytesPerElement: cfg.BytesPerElement,
		Slots:           cfg.Slots,
	}

	return CalculateVRAM(input), nil
}

// =============================================================================

// VRAMInput contains all parameters needed to calculate VRAM requirements.
type VRAMInput struct {
	ModelSizeBytes  int64 // Size of model weights in bytes
	ContextWindow   int64 // n_ctx - context window size (e.g., 8192, 131072)
	BlockCount      int64 // n_layers - number of transformer layers
	HeadCountKV     int64 // Number of KV attention heads
	KeyLength       int64 // K dimension per head (typically 128)
	ValueLength     int64 // V dimension per head (typically 128)
	BytesPerElement int64 // Depends on cache type: q8_0=1, f16=2
	Slots           int64 // n_seq_max - number of concurrent sequences
}

// CalculateVRAM computes the VRAM requirements for running a model based on
// the provided input parameters.
func CalculateVRAM(input VRAMInput) VRAM {

	// Calculate bytes needed per token in each transformer layer.
	// Formula: head_count_kv × (key_length + value_length) × bytes_per_element
	// Example: 4 × (128 + 128) × 1 = 1024 bytes
	kvPerTokenPerLayer := input.HeadCountKV * (input.KeyLength + input.ValueLength) * input.BytesPerElement

	// Calculate total KV cache memory per slot (sequence).
	// Formula: n_ctx × n_layers × kv_per_token_per_layer
	// Example: 131072 × 48 × 1024 = ~6.4 GB
	kvPerSlot := input.ContextWindow * input.BlockCount * kvPerTokenPerLayer

	// Total KV cache memory allocated at model load time.
	// Formula: slots × kv_per_slot
	// Example: 2 × 6.4GB = ~12.8 GB
	slotMemory := input.Slots * kvPerSlot

	// Total VRAM = model weights + KV cache memory.
	// Example: 36GB + 12.8GB = ~48.8 GB
	totalVRAM := input.ModelSizeBytes + slotMemory

	return VRAM{
		Input:              input,
		KVPerTokenPerLayer: kvPerTokenPerLayer,
		KVPerSlot:          kvPerSlot,
		SlotMemory:         slotMemory,
		TotalVRAM:          totalVRAM,
	}
}

// =============================================================================

// CalculateVRAMFromHuggingFace fetches GGUF metadata from HuggingFace using HTTP
// Range requests and calculates VRAM requirements. Only the header is downloaded,
// not the entire model file.
func CalculateVRAMFromHuggingFace(ctx context.Context, modelURL string, cfg VRAMConfig) (VRAM, error) {
	modelURL = NormalizeHuggingFaceDownloadURL(modelURL)

	metadata, fileSize, err := FetchGGUFMetadata(ctx, modelURL)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to fetch GGUF metadata: %w", err)
	}

	arch := detectArchitecture(metadata)
	if arch == "" {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: unable to detect model architecture")
	}

	if isVisionEncoder(arch) {
		return VRAM{
			Input:     VRAMInput{ModelSizeBytes: fileSize},
			TotalVRAM: fileSize,
		}, nil
	}

	blockCount, err := parseMetadataInt64WithFallback(metadata, arch+".block_count", ".block_count")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to parse block_count: %w", err)
	}

	headCountKV, err := parseMetadataInt64(metadata, arch+".attention.head_count_kv")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to parse head_count_kv: %w", err)
	}

	keyLength, err := parseMetadataInt64(metadata, arch+".attention.key_length")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to parse key_length: %w", err)
	}

	valueLength, err := parseMetadataInt64(metadata, arch+".attention.value_length")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to parse value_length: %w", err)
	}

	input := VRAMInput{
		ModelSizeBytes:  fileSize,
		ContextWindow:   cfg.ContextWindow,
		BlockCount:      blockCount,
		HeadCountKV:     headCountKV,
		KeyLength:       keyLength,
		ValueLength:     valueLength,
		BytesPerElement: cfg.BytesPerElement,
		Slots:           cfg.Slots,
	}

	return CalculateVRAM(input), nil
}

// =============================================================================

func detectArchitecture(metadata map[string]string) string {
	if arch, ok := metadata["general.architecture"]; ok {
		return arch
	}
	return ""
}

func isVisionEncoder(arch string) bool {
	switch arch {
	case "clip", "qwen2vl":
		return true
	}
	return false
}

func parseMetadataInt64(metadata map[string]string, key string) (int64, error) {
	val, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("parse-metadata-int64: metadata key %q not found", key)
	}
	return strconv.ParseInt(val, 10, 64)
}

func parseMetadataInt64WithFallback(metadata map[string]string, key string, suffix string) (int64, error) {
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

// FetchGGUFMetadata fetches GGUF header and metadata using HTTP Range requests.
func FetchGGUFMetadata(ctx context.Context, url string) (map[string]string, int64, error) {
	var client http.Client

	initialBytes := 24
	headerData, fileSize, err := fetchRange(ctx, &client, url, 0, int64(initialBytes-1))
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: failed to fetch initial header: %w", err)
	}

	reader := bytes.NewReader(headerData)

	var header ggufHeader
	if err := binary.Read(reader, binary.LittleEndian, &header.Magic); err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: failed to read magic: %w", err)
	}

	if header.Magic != ggufMagic {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: invalid GGUF magic number: got 0x%X", header.Magic)
	}

	if err := binary.Read(reader, binary.LittleEndian, &header.Version); err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: failed to read version: %w", err)
	}

	if err := binary.Read(reader, binary.LittleEndian, &header.TensorCount); err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: failed to read tensor count: %w", err)
	}

	if err := binary.Read(reader, binary.LittleEndian, &header.MetadataKvCount); err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: failed to read metadata count: %w", err)
	}

	metadataSize := estimateMetadataSize(header.MetadataKvCount)
	metadataData, _, err := fetchRange(ctx, &client, url, int64(initialBytes), int64(initialBytes)+metadataSize-1)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: failed to fetch metadata: %w", err)
	}

	allData := append(headerData, metadataData...)
	fullReader := bytes.NewReader(allData)
	fullReader.Seek(int64(initialBytes), io.SeekStart)

	metadata := make(map[string]string)
	for i := uint64(0); i < header.MetadataKvCount; i++ {
		key, value, err := readMetadataKVFromReader(fullReader)
		if err != nil {
			break
		}
		metadata[key] = fmt.Sprintf("%v", value)
	}

	return metadata, fileSize, nil
}

// fetchRange fetches a byte range from a URL using HTTP Range requests.
func fetchRange(ctx context.Context, client *http.Client, url string, start, end int64) ([]byte, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}

	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))

	if token := os.Getenv("KRONK_HF_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("fetch-range: unexpected status code: %d", resp.StatusCode)
	}

	cr := resp.Header.Get("Content-Range")

	var fileSize int64
	switch {
	case cr != "":
		var start, end int64
		fmt.Sscanf(cr, "bytes %d-%d/%d", &start, &end, &fileSize)

	case resp.ContentLength > 0 && resp.StatusCode == http.StatusOK:
		fileSize = resp.ContentLength
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, 0, err
	}

	return data, fileSize, nil
}

// estimateMetadataSize estimates how many bytes to fetch for metadata.
func estimateMetadataSize(kvCount uint64) int64 {
	return int64(kvCount * 512)
}

// readMetadataKVFromReader reads a key-value pair from a bytes.Reader.
func readMetadataKVFromReader(r *bytes.Reader) (string, any, error) {
	var keyLen uint64
	if err := binary.Read(r, binary.LittleEndian, &keyLen); err != nil {
		return "", nil, err
	}

	if keyLen > 1*1024*1024 {
		return "", nil, fmt.Errorf("read-metadata-kvf-from-reader: key length too large: %d", keyLen)
	}

	keyBytes := make([]byte, keyLen)
	if _, err := io.ReadFull(r, keyBytes); err != nil {
		return "", nil, err
	}
	key := string(keyBytes)

	var valueType uint32
	if err := binary.Read(r, binary.LittleEndian, &valueType); err != nil {
		return "", nil, err
	}

	value, err := readMetadataValueFromReader(r, valueType)
	if err != nil {
		return key, nil, err
	}

	return key, value, nil
}

// readMetadataValueFromReader reads a metadata value from a bytes.Reader.
func readMetadataValueFromReader(r *bytes.Reader, valueType uint32) (interface{}, error) {
	switch valueType {
	case ggufMetadataValueTypeUInt8:
		var val uint8
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeInt8:
		var val int8
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeUInt16:
		var val uint16
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeInt16:
		var val int16
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeUInt32:
		var val uint32
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeInt32:
		var val int32
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeFloat32:
		var val float32
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeBool:
		var val uint8
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val != 0, nil

	case ggufMetadataValueTypeString:
		var strLen uint64
		if err := binary.Read(r, binary.LittleEndian, &strLen); err != nil {
			return nil, err
		}

		if strLen > 1*1024*1024 {
			return nil, fmt.Errorf("string length too large: %d", strLen)
		}

		strBytes := make([]byte, strLen)
		if _, err := io.ReadFull(r, strBytes); err != nil {
			return nil, err
		}
		return string(strBytes), nil

	case ggufMetadataValueTypeArray:
		var arrayType uint32
		if err := binary.Read(r, binary.LittleEndian, &arrayType); err != nil {
			return nil, err
		}

		var arrayLen uint64
		if err := binary.Read(r, binary.LittleEndian, &arrayLen); err != nil {
			return nil, err
		}

		result := make([]any, arrayLen)
		for i := uint64(0); i < arrayLen; i++ {
			val, err := readMetadataValueFromReader(r, arrayType)
			if err != nil {
				return nil, err
			}
			result[i] = val
		}
		return result, nil

	case ggufMetadataValueTypeUInt64:
		var val uint64
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeInt64:
		var val int64
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeFloat64:
		var val float64
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	default:
		return nil, fmt.Errorf("unsupported metadata value type: %d", valueType)
	}
}

// =============================================================================
/*
SLOT MEMORY AND TOTAL VRAM COST FORMULA

These figures are for KV cache VRAM only (when offload-kqv: true).
Model weights require additional VRAM: ~7GB (7B Q8) or ~70GB (70B Q8).
Total VRAM = model weights + KV cache.

Memory is statically allocated upfront when the model loads,
based on n_ctx × n_seq_max. Reserving slots consumes memory whether or not
they're actually used.

Example Calculations:

This is how you calculate the amount of KV memory you need per slot.

KV_Per_Token_Per_Layer = head_count_kv × (key_length + value_length) × bytes_per_element
KV_Per_Slot            = n_ctx × n_layers × KV_per_token_per_layer

------------------------------------------------------------------------------
So Given these values, this is what you are looking at:

Model   Context_Window   KV_Per_Slot      NSeqMax (Slots)
7B      8K               ~537 MB VRAM     2
70B     8K               ~1.3 GB VRAM     2

Total sequences allocated: 2
7B:  Slot Memory (2 × 537MB) ~1.07GB: Total VRAM: ~8.1GB
70B: Slot Memory (2 × 1.3GB) ~2.6GB : Total VRAM: ~72.6GB

Cache type (off, SPC, IMC) does not affect VRAM. All modes allocate
the same slots and KV cache.

------------------------------------------------------------------------------
Full Example With Real Model:

Model                   : Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
Size                    : 36.0GB
Context Window          : 131072 (128k)
cache-type-k            : q8_0 (1 byte per element), f16 (2 bytes)
cache-type-v            : q8_0 (1 byte per element), f16 (2 bytes)
block_count             : 48  (n_layers)
attention.head_count_kv : 4   (KV heads)
attention.key_length    : 128	(K dimension per head)
attention.value_length  : 128	(V dimension per head)

KV_per_token_per_layer = head_count_kv  ×  (key_length + value_length)  ×  bytes_per_element
1024 bytes             =             4  ×  ( 128       +         128 )  ×  1

KV_Per_Slot            =  n_ctx  ×  n_layers  ×  KV_per_token_per_layer
~6.4 GB                =  131072 ×  48        ×  1024

Total sequences allocated: 2
Slot Memory (2 × 6.4GB) ~12.8GB: Total VRAM: ~48.8GB

Cache type (off, SPC, IMC) does not affect VRAM. All modes allocate
the same slots and KV cache.
*/
