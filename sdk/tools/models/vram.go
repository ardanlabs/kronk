package models

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
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
//
// The modelURL can be either:
//   - A single file URL: https://huggingface.co/org/repo/resolve/main/model.gguf
//   - A folder URL for split models: https://huggingface.co/org/repo/tree/main/UD-Q5_K_XL
func CalculateVRAMFromHuggingFace(ctx context.Context, modelURL string, cfg VRAMConfig) (VRAM, error) {
	if isHuggingFaceFolderURL(modelURL) {
		return calculateVRAMFromHuggingFaceFolder(ctx, modelURL, cfg)
	}

	modelURL = NormalizeHuggingFaceDownloadURL(modelURL)

	metadata, fileSize, err := FetchGGUFMetadata(ctx, modelURL)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to fetch GGUF metadata: %w", err)
	}

	return buildVRAMFromMetadata(metadata, fileSize, cfg)
}

// calculateVRAMFromHuggingFaceFolder handles VRAM calculation for split models
// hosted in a HuggingFace folder. It lists all GGUF files in the folder, sums
// their sizes, and reads metadata from the first split file.
func calculateVRAMFromHuggingFaceFolder(ctx context.Context, folderURL string, cfg VRAMConfig) (VRAM, error) {
	fileURLs, totalSize, err := fetchHuggingFaceFolderFiles(ctx, folderURL)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: %w", err)
	}

	metadata, _, err := FetchGGUFMetadata(ctx, fileURLs[0])
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to fetch GGUF metadata from split: %w", err)
	}

	return buildVRAMFromMetadata(metadata, totalSize, cfg)
}

// buildVRAMFromMetadata extracts model parameters from GGUF metadata and
// computes the VRAM requirements.
func buildVRAMFromMetadata(metadata map[string]string, modelSizeBytes int64, cfg VRAMConfig) (VRAM, error) {
	arch := detectArchitecture(metadata)
	if arch == "" {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: unable to detect model architecture")
	}

	if isVisionEncoder(arch) {
		return VRAM{
			Input:     VRAMInput{ModelSizeBytes: modelSizeBytes},
			TotalVRAM: modelSizeBytes,
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
		ModelSizeBytes:  modelSizeBytes,
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

// isHuggingFaceFolderURL returns true if the URL points to a HuggingFace
// folder containing split model files rather than a single GGUF file.
func isHuggingFaceFolderURL(modelURL string) bool {
	if strings.Contains(modelURL, "/tree/") {
		return true
	}

	lower := strings.ToLower(modelURL)
	if !strings.HasSuffix(lower, ".gguf") && !strings.Contains(modelURL, "/resolve/") {
		return true
	}

	return false
}

// hfTreeEntry represents a file entry returned by the HuggingFace tree API.
type hfTreeEntry struct {
	Type string `json:"type"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	LFS  *struct {
		Size int64 `json:"size"`
	} `json:"lfs"`
}

// fetchHuggingFaceFolderFiles lists GGUF files in a HuggingFace folder and
// returns their download URLs (sorted) and total size.
func fetchHuggingFaceFolderFiles(ctx context.Context, folderURL string) ([]string, int64, error) {
	owner, repo, folderPath, err := parseHuggingFaceFolderURL(folderURL)
	if err != nil {
		return nil, 0, err
	}

	apiURL := fmt.Sprintf("https://huggingface.co/api/models/%s/%s/tree/main/%s", owner, repo, folderPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-hf-folder-files: creating request: %w", err)
	}

	if token := os.Getenv("KRONK_HF_TOKEN"); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-hf-folder-files: fetching: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("fetch-hf-folder-files: unexpected status %d for %s", resp.StatusCode, apiURL)
	}

	var entries []hfTreeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, 0, fmt.Errorf("fetch-hf-folder-files: decoding: %w", err)
	}

	var fileURLs []string
	var totalSize int64

	for _, entry := range entries {
		if entry.Type != "file" {
			continue
		}
		if !strings.HasSuffix(strings.ToLower(entry.Path), ".gguf") {
			continue
		}

		size := entry.Size
		if entry.LFS != nil {
			size = entry.LFS.Size
		}

		downloadURL := fmt.Sprintf("https://huggingface.co/%s/%s/resolve/main/%s", owner, repo, entry.Path)
		fileURLs = append(fileURLs, downloadURL)
		totalSize += size
	}

	if len(fileURLs) == 0 {
		return nil, 0, fmt.Errorf("fetch-hf-folder-files: no GGUF files found in folder %s/%s/%s", owner, repo, folderPath)
	}

	slices.Sort(fileURLs)

	return fileURLs, totalSize, nil
}

// parseHuggingFaceFolderURL extracts owner, repo, and folder path from a
// HuggingFace folder URL.
//
// Supported formats:
//
//	https://huggingface.co/owner/repo/tree/main/subfolder
//	owner/repo/tree/main/subfolder
//	owner/repo/subfolder (no /tree/main/ prefix)
func parseHuggingFaceFolderURL(folderURL string) (owner, repo, folderPath string, err error) {
	raw := folderURL
	raw = strings.TrimPrefix(raw, "https://huggingface.co/")
	raw = strings.TrimPrefix(raw, "http://huggingface.co/")

	parts := strings.SplitN(raw, "/", 3)
	if len(parts) < 3 {
		return "", "", "", fmt.Errorf("parse-hf-folder-url: invalid folder URL: %s", folderURL)
	}

	owner = parts[0]
	repo = parts[1]
	rest := parts[2]

	// Strip tree/main/ prefix if present.
	rest = strings.TrimPrefix(rest, "tree/main/")

	// Strip blob/main/ prefix if present.
	rest = strings.TrimPrefix(rest, "blob/main/")

	if rest == "" {
		return "", "", "", fmt.Errorf("parse-hf-folder-url: missing folder path in URL: %s", folderURL)
	}

	return owner, repo, rest, nil
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
func readMetadataValueFromReader(r *bytes.Reader, valueType uint32) (any, error) {
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
VRAM CALCULATION FORMULA

Total VRAM = Model Weights + KV Cache

Model weights are determined by the GGUF file size (e.g., ~8GB for a
7B Q8_0 model). The KV cache is the variable cost you control through
configuration.

SLOTS AND SEQUENCES

A slot is a processing unit that handles one request at a time. Each slot
is assigned a unique sequence ID that maps to an isolated partition in the
shared KV cache. The mapping is always 1:1:

  NSeqMax = 4 (set via n_seq_max in model config)

  Slot 0  →  Sequence 0  →  KV cache partition 0
  Slot 1  →  Sequence 1  →  KV cache partition 1
  Slot 2  →  Sequence 2  →  KV cache partition 2
  Slot 3  →  Sequence 3  →  KV cache partition 3

NSeqMax controls how many slots (and sequences) are created. More slots
means more concurrent requests, but each slot reserves its own KV cache
partition in VRAM whether or not it is actively used.

WHAT AFFECTS KV CACHE MEMORY PER SEQUENCE

Each sequence's KV cache partition size is determined by three factors:

  1. Context Window (n_ctx) — larger context linearly increases memory.
  2. Number of Layers (block_count) — more layers means more memory per token.
  3. KV Cache Precision (bytes_per_element) — f16=2 bytes, q8_0=1 byte.

The head geometry (head_count_kv, key_length, value_length) is fixed by
the model architecture and read from the GGUF header.

  KV_Per_Token_Per_Layer = head_count_kv × (key_length + value_length) × bytes_per_element
  KV_Per_Sequence        = n_ctx × n_layers × KV_Per_Token_Per_Layer

WHAT AFFECTS TOTAL KV CACHE (SLOT MEMORY)

  Slot_Memory = TotalSequences × KV_Per_Sequence
  Total_VRAM  = Model_Weights + Slot_Memory

Memory is statically allocated upfront when the model loads.

CACHING MODES (SPC / IMC)

Neither SPC nor IMC adds extra sequences to the VRAM calculation.

SPC (System Prompt Cache) externalizes the decoded system prompt KV
state to a byte buffer in RAM. On each request, the KV state is
restored into the slot's sequence via StateSeqSetData. No dedicated
cache sequence is permanently occupied.

IMC (Incremental Message Cache) uses dedicated slot/seq binding —
each slot's sequence IS the cache. No separate cache sequences.

EXAMPLE

Model                   : Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
Model Weights           : 36.0 GB
Context Window (n_ctx)  : 131,072 (128K)
cache-type-k / v        : q8_0 (1 byte per element)
block_count (n_layers)  : 48
attention.head_count_kv : 4
attention.key_length    : 128
attention.value_length  : 128

KV_Per_Token_Per_Layer = 4 × (128 + 128) × 1       = 1,024 bytes
KV_Per_Sequence        = 131,072 × 48 × 1,024      = ~6.4 GB
Slot_Memory            = 2 × 6.4 GB                 = ~12.8 GB
Total_VRAM             = 36.0 GB + 12.8 GB           = ~48.8 GB
*/
