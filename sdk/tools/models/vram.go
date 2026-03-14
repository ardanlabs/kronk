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
	MoE                *MoEInfo
	Weights            *WeightBreakdown
	ModelWeightsGPU    int64
	ModelWeightsCPU    int64
	ComputeBufferEst   int64
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

	headCountKV, err := parseMetadataInt64OrArrayAvg(info.Metadata, arch+".attention.head_count_kv")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: failed to parse head_count_kv: %w", err)
	}

	keyLength, valueLength, err := resolveKVLengths(info.Metadata, arch)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: %w", err)
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
	ModelSizeBytes    int64            // Size of model weights in bytes
	ContextWindow     int64            // n_ctx - context window size (e.g., 8192, 131072)
	BlockCount        int64            // n_layers - number of transformer layers
	HeadCountKV       int64            // Number of KV attention heads
	KeyLength         int64            // K dimension per head (typically 128)
	ValueLength       int64            // V dimension per head (typically 128)
	BytesPerElement   int64            // Depends on cache type: q8_0=1, f16=2
	Slots             int64            // n_seq_max - number of concurrent sequences
	EmbeddingLength   int64            // needed for compute buffer estimate
	MoE               *MoEInfo         //
	Weights           *WeightBreakdown //
	GPULayers         int64            // Number of layers on GPU (0 or -1 = all layers)
	ExpertLayersOnGPU int64            // 0 = all experts on CPU
}

// CalculateVRAM computes the VRAM requirements for running a model based on
// the provided input parameters.
func CalculateVRAM(input VRAMInput) VRAM {
	kvPerTokenPerLayer := input.HeadCountKV * (input.KeyLength + input.ValueLength) * input.BytesPerElement
	kvPerSlot := input.ContextWindow * input.BlockCount * kvPerTokenPerLayer
	slotMemory := input.Slots * kvPerSlot

	gpuLayers := clampGPULayers(input.GPULayers, input.BlockCount)

	var modelWeightsGPU, modelWeightsCPU int64

	switch {
	case input.Weights != nil && input.MoE != nil && input.MoE.IsMoE:

		// Always-active weights are split proportionally by GPU layers.
		// When all layers are on GPU, all always-active weights stay on GPU.
		var alwaysActiveGPU, alwaysActiveCPU int64
		if gpuLayers >= input.BlockCount {
			alwaysActiveGPU = input.Weights.AlwaysActiveBytes
		} else {
			alwaysActiveGPU, alwaysActiveCPU = splitByGPULayers(input.Weights.AlwaysActiveBytes, gpuLayers, input.BlockCount)
		}

		// Expert weights are split by ExpertLayersOnGPU (expert offloading).
		var expertsGPU int64
		if input.ExpertLayersOnGPU > 0 && len(input.Weights.ExpertBytesByLayer) > 0 {
			blockCount := int64(len(input.Weights.ExpertBytesByLayer))
			startLayer := max(blockCount-input.ExpertLayersOnGPU, 0)
			for i := startLayer; i < blockCount; i++ {
				expertsGPU += input.Weights.ExpertBytesByLayer[i]
			}
		}

		modelWeightsGPU = alwaysActiveGPU + expertsGPU
		modelWeightsCPU = alwaysActiveCPU + max(0, input.Weights.ExpertBytesTotal-expertsGPU)

	default:

		// Dense models: split total model weights proportionally by GPU layers.
		if gpuLayers >= input.BlockCount {
			modelWeightsGPU = input.ModelSizeBytes
		} else {
			modelWeightsGPU, modelWeightsCPU = splitByGPULayers(input.ModelSizeBytes, gpuLayers, input.BlockCount)
		}
	}

	computeBufferEst := estimateComputeBuffer(input)
	totalVRAM := modelWeightsGPU + slotMemory + computeBufferEst

	return VRAM{
		Input:              input,
		KVPerTokenPerLayer: kvPerTokenPerLayer,
		KVPerSlot:          kvPerSlot,
		SlotMemory:         slotMemory,
		TotalVRAM:          totalVRAM,
		MoE:                input.MoE,
		Weights:            input.Weights,
		ModelWeightsGPU:    modelWeightsGPU,
		ModelWeightsCPU:    modelWeightsCPU,
		ComputeBufferEst:   computeBufferEst,
	}
}

// clampGPULayers returns the effective number of GPU layers. A zero value
// (the default) or -1 means all layers on GPU, preserving backward
// compatibility with callers that don't set GPULayers.
func clampGPULayers(gpuLayers, blockCount int64) int64 {
	if gpuLayers <= 0 || gpuLayers > blockCount {
		return blockCount
	}

	return gpuLayers
}

// splitByGPULayers splits totalBytes proportionally between GPU and CPU based
// on how many layers are offloaded.
func splitByGPULayers(totalBytes, gpuLayers, blockCount int64) (gpu, cpu int64) {
	if blockCount <= 0 {
		return totalBytes, 0
	}

	gpu = (gpuLayers * totalBytes) / blockCount
	cpu = max(0, totalBytes-gpu)

	return gpu, cpu
}

// estimateComputeBuffer provides a heuristic estimate of the compute buffer
// VRAM needed during inference. This is inherently approximate.
func estimateComputeBuffer(input VRAMInput) int64 {
	const (
		baseBufferSmall = 256 * 1024 * 1024 // 256 MiB for models < 100B params
		baseBufferLarge = 512 * 1024 * 1024 // 512 MiB for models >= 100B params
		k               = 8                 // empirical multiplier
	)

	baseBuffer := int64(baseBufferSmall)
	if input.ModelSizeBytes > 50*1024*1024*1024 {
		baseBuffer = int64(baseBufferLarge)
	}

	var embeddingComponent int64
	if input.EmbeddingLength > 0 {
		nUBatch := int64(512)
		embeddingComponent = k * nUBatch * input.EmbeddingLength * 4
	}

	total := baseBuffer + embeddingComponent
	total = total + total/10

	return total
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

	metadata, tensors, fileSize, err := fetchGGUFHeaderAndTensors(ctx, modelURL)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to fetch GGUF metadata: %w", err)
	}

	return buildVRAMFromMetadata(metadata, tensors, fileSize, cfg)
}

// calculateVRAMFromHuggingFaceFolder handles VRAM calculation for split models
// hosted in a HuggingFace folder. It lists all GGUF files in the folder, sums
// their sizes, and reads metadata from the first split file.
func calculateVRAMFromHuggingFaceFolder(ctx context.Context, folderURL string, cfg VRAMConfig) (VRAM, error) {
	fileURLs, totalSize, err := fetchHuggingFaceFolderFiles(ctx, folderURL)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: %w", err)
	}

	metadata, tensors, _, err := fetchGGUFHeaderAndTensors(ctx, fileURLs[0])
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to fetch GGUF metadata from split: %w", err)
	}

	return buildVRAMFromMetadata(metadata, tensors, totalSize, cfg)
}

// buildVRAMFromMetadata extracts model parameters from GGUF metadata and
// computes the VRAM requirements. When tensors is non-nil, a WeightBreakdown
// is computed and attached to the result.
func buildVRAMFromMetadata(metadata map[string]string, tensors []ggufTensorInfo, modelSizeBytes int64, cfg VRAMConfig) (VRAM, error) {
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

	headCountKV, err := parseMetadataInt64OrArrayAvg(metadata, arch+".attention.head_count_kv")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to parse head_count_kv: %w", err)
	}

	keyLength, valueLength, err := resolveKVLengths(metadata, arch)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: %w", err)
	}

	embeddingLength, _ := parseMetadataInt64WithFallback(metadata, arch+".embedding_length", ".embedding_length")

	moeInfo := detectMoE(metadata)
	var moePtr *MoEInfo
	if moeInfo.IsMoE {
		moePtr = &moeInfo
	}

	var weights *WeightBreakdown
	if len(tensors) > 0 {
		wb := categorizeWeights(tensors, blockCount)
		weights = &wb
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
		EmbeddingLength: embeddingLength,
		MoE:             moePtr,
		Weights:         weights,
	}

	return CalculateVRAM(input), nil
}

// CalculateVRAMFromHuggingFaceFiles computes VRAM requirements from a set of
// pre-resolved HuggingFace file URLs (e.g. from shorthand resolution). It reads
// metadata from the first file and sums sizes across all files for split models.
func CalculateVRAMFromHuggingFaceFiles(ctx context.Context, modelURLs []string, cfg VRAMConfig) (VRAM, error) {
	if len(modelURLs) == 0 {
		return VRAM{}, fmt.Errorf("calculate-vram-hg-files: no model URLs provided")
	}

	normalized := make([]string, len(modelURLs))
	for i, u := range modelURLs {
		normalized[i] = NormalizeHuggingFaceDownloadURL(u)
	}

	metadata, tensors, firstSize, err := fetchGGUFHeaderAndTensors(ctx, normalized[0])
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg-files: failed to fetch GGUF metadata: %w", err)
	}

	totalSize := firstSize
	if len(normalized) > 1 {
		var client http.Client
		for i := 1; i < len(normalized); i++ {
			_, splitSize, err := fetchRange(ctx, &client, normalized[i], 0, 0)
			if err != nil {
				return VRAM{}, fmt.Errorf("calculate-vram-hg-files: failed to determine size for %s: %w", normalized[i], err)
			}
			totalSize += splitSize
		}
	}

	return buildVRAMFromMetadata(metadata, tensors, totalSize, cfg)
}

// isHuggingFaceFolderURL returns true if the URL points to a HuggingFace
// folder containing split model files rather than a single GGUF file.
func isHuggingFaceFolderURL(modelURL string) bool {
	if strings.Contains(modelURL, "/tree/") {
		return true
	}

	lower := strings.ToLower(modelURL)
	if strings.HasSuffix(lower, ".gguf") || strings.Contains(lower, "/resolve/") || strings.Contains(lower, "/blob/") {
		return false
	}

	// Strip HF host prefix and scheme for path segment counting.
	raw := modelURL
	for _, prefix := range []string{
		"https://huggingface.co/",
		"http://huggingface.co/",
		"https://hf.co/",
		"http://hf.co/",
	} {
		if strings.HasPrefix(strings.ToLower(raw), prefix) {
			raw = raw[len(prefix):]
			break
		}
	}
	raw = stripHFHostPrefix(raw)

	// Shorthand like "owner/repo:TAG" has a colon — not a folder URL.
	if strings.Contains(raw, ":") {
		return false
	}

	// 3+ path segments (owner/repo/subfolder) indicates a folder.
	parts := strings.Split(raw, "/")
	return len(parts) >= 3
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

// resolveKVLengths returns key_length and value_length for VRAM calculation.
// It first checks for explicit metadata keys. When those are missing (e.g. LFM2
// hybrid models), it falls back to embedding_length / head_count which is the
// same default llama.cpp uses internally.
func resolveKVLengths(metadata map[string]string, arch string) (keyLen int64, valLen int64, err error) {
	keyLen, keyErr := parseMetadataInt64(metadata, arch+".attention.key_length")
	valLen, valErr := parseMetadataInt64(metadata, arch+".attention.value_length")

	if keyErr == nil && valErr == nil {
		return keyLen, valLen, nil
	}

	// Fallback: embedding_length / head_count.
	embLen, err := parseMetadataInt64(metadata, arch+".embedding_length")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to derive key/value lengths: key_length and embedding_length both missing")
	}

	headCount, err := parseMetadataInt64(metadata, arch+".attention.head_count")
	if err != nil {
		return 0, 0, fmt.Errorf("failed to derive key/value lengths: key_length and head_count both missing")
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

func parseMetadataInt64(metadata map[string]string, key string) (int64, error) {
	val, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("parse-metadata-int64: metadata key %q not found", key)
	}
	return strconv.ParseInt(val, 10, 64)
}

// parseMetadataInt64OrArrayAvg parses a metadata value that may be either a
// single integer (e.g. "8") or a per-layer array (e.g. "[0 0 8 0 0 8 ...]").
// For arrays, the average of all elements is returned. This handles hybrid
// architectures like LFM2 where head_count_kv varies per layer.
func parseMetadataInt64OrArrayAvg(metadata map[string]string, key string) (int64, error) {
	val, ok := metadata[key]
	if !ok {
		return 0, fmt.Errorf("parse-metadata-int64: metadata key %q not found", key)
	}

	// Try scalar first.
	if n, err := strconv.ParseInt(val, 10, 64); err == nil {
		return n, nil
	}

	// Try array format: "[v1 v2 v3 ...]" produced by fmt.Sprintf("%v", []any{...}).
	trimmed := strings.TrimSpace(val)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return 0, fmt.Errorf("parse-metadata-int64: unable to parse %q for key %q", val, key)
	}

	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return 0, fmt.Errorf("parse-metadata-int64: empty array for key %q", key)
	}

	fields := strings.Fields(inner)

	var sum int64
	for _, f := range fields {
		n, err := strconv.ParseInt(f, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse-metadata-int64: unable to parse array element %q for key %q: %w", f, key, err)
		}
		sum += n
	}

	return sum / int64(len(fields)), nil
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
	data, fileSize, err := fetchGGUFHeaderBytes(ctx, url)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: failed to fetch header data: %w", err)
	}

	reader := bytes.NewReader(data)

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

	metadata := make(map[string]string)
	for i := uint64(0); i < header.MetadataKvCount; i++ {
		key, value, err := readMetadataKVFromReader(reader)
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
		return nil, 0, fmt.Errorf("fetch-range: unexpected status code: %d, url=%s", resp.StatusCode, resp.Request.URL.Host)
	}

	cr := resp.Header.Get("Content-Range")

	var (
		fileSize  int64
		respStart int64
		respEnd   int64
		haveRange bool
	)

	if cr != "" {
		if n, _ := fmt.Sscanf(cr, "bytes %d-%d/%d", &respStart, &respEnd, &fileSize); n == 3 {
			haveRange = true
		}
	} else if resp.ContentLength > 0 && resp.StatusCode == http.StatusOK {
		fileSize = resp.ContentLength
	}

	// When the server returns 200 OK (full file) instead of 206 Partial
	// Content, read only the requested range to avoid downloading the
	// entire file. This happens when HuggingFace redirects to a storage
	// backend (e.g., Xet) that does not support HTTP Range requests.
	var reader io.Reader = resp.Body
	if resp.StatusCode == http.StatusOK {
		if start > 0 {
			if _, err := io.CopyN(io.Discard, resp.Body, start); err != nil {
				return nil, 0, fmt.Errorf("fetch-range: failed to skip to offset %d: %w", start, err)
			}
		}
		reader = io.LimitReader(resp.Body, end-start+1)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-range: read body failed: status=%d, requested_range=%d-%d, content_range=%q, content_length=%d, host=%s: %w",
			resp.StatusCode, start, end, cr, resp.ContentLength, resp.Request.URL.Host, err)
	}

	if resp.StatusCode == http.StatusPartialContent {
		switch {
		case haveRange:
			// When the requested range extends past EOF the server returns the
			// satisfiable subrange. Clamp our expectation to match.
			expectedEnd := end
			if fileSize > 0 && expectedEnd >= fileSize {
				expectedEnd = fileSize - 1
			}

			if respStart != start || respEnd != expectedEnd {
				return nil, 0, fmt.Errorf("fetch-range: unexpected content-range: requested=%d-%d, got=%q, host=%s",
					start, end, cr, resp.Request.URL.Host)
			}

			expectedLen := respEnd - respStart + 1
			if int64(len(data)) != expectedLen {
				return nil, 0, fmt.Errorf("fetch-range: short read: got %d bytes, expected %d, status=%d, content_range=%q, host=%s",
					len(data), expectedLen, resp.StatusCode, cr, resp.Request.URL.Host)
			}

		default:
			// No parseable Content-Range; fall back to the original check.
			if int64(len(data)) < end-start+1 {
				return nil, 0, fmt.Errorf("fetch-range: short read: got %d bytes, expected %d, status=%d, content_range=%q, host=%s",
					len(data), end-start+1, resp.StatusCode, cr, resp.Request.URL.Host)
			}
		}
	}

	return data, fileSize, nil
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
