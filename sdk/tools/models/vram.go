package models

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
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

// Bytes per element constants for KV cache types. Re-exported from
// sdk/kronk/gguf so existing callers (analyze.go, plan.go, downstream
// code) keep their import surface.
const (
	BytesPerElementF32  = gguf.BytesPerElementF32
	BytesPerElementF16  = gguf.BytesPerElementF16
	BytesPerElementBF16 = gguf.BytesPerElementBF16
	BytesPerElementQ8_0 = gguf.BytesPerElementQ8_0
	BytesPerElementQ4_0 = gguf.BytesPerElementQ4_0
	BytesPerElementQ4_1 = gguf.BytesPerElementQ4_1
	BytesPerElementQ5_0 = gguf.BytesPerElementQ5_0
	BytesPerElementQ5_1 = gguf.BytesPerElementQ5_1
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

	arch := gguf.DetectArchitecture(info.Metadata)
	if arch == "" {
		return VRAM{}, fmt.Errorf("calculate-vram: unable to detect model architecture")
	}

	if gguf.IsVisionEncoder(arch) {
		return VRAM{
			Input:     VRAMInput{ModelSizeBytes: int64(info.Size)},
			TotalVRAM: int64(info.Size),
		}, nil
	}

	blockCount, err := gguf.ParseInt64WithFallback(info.Metadata, arch+".block_count", ".block_count")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: failed to parse block_count: %w", err)
	}

	headCountKV, err := gguf.ParseInt64OrArrayAvg(info.Metadata, arch+".attention.head_count_kv")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram: failed to parse head_count_kv: %w", err)
	}

	keyLength, valueLength, err := gguf.ResolveKVLengths(info.Metadata, arch)
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
// the provided input parameters. The KV cache portion of the math is
// delegated to sdk/kronk/gguf.CalculateKVCache so the SDK and tools sides
// share a single implementation.
func CalculateVRAM(input VRAMInput) VRAM {
	kv := gguf.CalculateKVCache(gguf.KVCacheInput{
		ContextWindow:   input.ContextWindow,
		BlockCount:      input.BlockCount,
		HeadCountKV:     input.HeadCountKV,
		KeyLength:       input.KeyLength,
		ValueLength:     input.ValueLength,
		BytesPerElement: input.BytesPerElement,
		Slots:           input.Slots,
	})
	kvPerTokenPerLayer := kv.KVPerTokenPerLayer
	kvPerSlot := kv.KVPerSlot
	slotMemory := kv.SlotMemory

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
// is computed and attached to the result. Tensor parsing and weight
// categorization are delegated to sdk/kronk/gguf; the result is
// translated into the models-side WeightBreakdown / MoEInfo so the public
// VRAM/VRAMInput API does not leak the gguf type.
func buildVRAMFromMetadata(metadata map[string]string, tensors []gguf.TensorInfo, modelSizeBytes int64, cfg VRAMConfig) (VRAM, error) {
	arch := gguf.DetectArchitecture(metadata)
	if arch == "" {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: unable to detect model architecture")
	}

	if gguf.IsVisionEncoder(arch) {
		return VRAM{
			Input:     VRAMInput{ModelSizeBytes: modelSizeBytes},
			TotalVRAM: modelSizeBytes,
		}, nil
	}

	blockCount, err := gguf.ParseInt64WithFallback(metadata, arch+".block_count", ".block_count")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to parse block_count: %w", err)
	}

	headCountKV, err := gguf.ParseInt64OrArrayAvg(metadata, arch+".attention.head_count_kv")
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: failed to parse head_count_kv: %w", err)
	}

	keyLength, valueLength, err := gguf.ResolveKVLengths(metadata, arch)
	if err != nil {
		return VRAM{}, fmt.Errorf("calculate-vram-hg: %w", err)
	}

	embeddingLength, _ := gguf.ParseInt64WithFallback(metadata, arch+".embedding_length", ".embedding_length")

	moeInfo := detectMoE(metadata)
	var moePtr *MoEInfo
	if moeInfo.IsMoE {
		moePtr = &moeInfo
	}

	var weights *WeightBreakdown
	if len(tensors) > 0 {
		wb := weightBreakdownFromGGUF(gguf.CategorizeWeights(tensors, blockCount))
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

// BuildVRAMFromBytes computes the VRAM requirements directly from
// already-fetched GGUF header bytes (typically the first
// gguf.HeaderFetchSize bytes from the catalog cache or a local file).
// totalSize is the on-disk size of all model files combined.
func BuildVRAMFromBytes(data []byte, totalSize int64, cfg VRAMConfig) (VRAM, error) {
	metadata, tensors, err := gguf.ParseHeaderAndTensors(data, totalSize)
	if err != nil {
		return VRAM{}, fmt.Errorf("build-vram-bytes: %w", err)
	}

	return buildVRAMFromMetadata(metadata, tensors, totalSize, cfg)
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
		for i := 1; i < len(normalized); i++ {
			_, splitSize, err := gguf.FetchRange(ctx, normalized[i], 0, 0)
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

// fetchHuggingFaceFolderFiles lists GGUF files in a HuggingFace folder and
// returns their download URLs (sorted) and total size.
func fetchHuggingFaceFolderFiles(ctx context.Context, folderURL string) ([]string, int64, error) {
	owner, repo, folderPath, err := parseHuggingFaceFolderURL(folderURL)
	if err != nil {
		return nil, 0, err
	}

	repoFiles, err := FetchHFRepoFiles(ctx, owner, repo, "main", folderPath, false)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-hf-folder-files: %w", err)
	}

	var fileURLs []string
	var totalSize int64

	for _, f := range repoFiles {
		if !strings.HasSuffix(strings.ToLower(f.Filename), ".gguf") {
			continue
		}

		downloadURL := fmt.Sprintf("https://huggingface.co/%s/%s/resolve/main/%s", owner, repo, f.Filename)
		fileURLs = append(fileURLs, downloadURL)
		totalSize += f.Size
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

// FetchGGUFMetadata fetches GGUF header and metadata using HTTP Range requests.
func FetchGGUFMetadata(ctx context.Context, url string) (map[string]string, int64, error) {
	data, fileSize, err := gguf.FetchHeaderBytes(ctx, url)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: failed to fetch header data: %w", err)
	}

	metadata, err := ParseGGUFMetadata(data)
	if err != nil {
		return nil, 0, fmt.Errorf("fetch-gguf-metadata: %w", err)
	}

	return metadata, fileSize, nil
}

// ParseGGUFMetadata parses the GGUF header and key-value metadata from a
// byte slice (typically the first gguf.HeaderFetchSize bytes of a GGUF
// file). Implementation lives in sdk/kronk/gguf; this wrapper preserves
// the existing public symbol for cmd/ callers.
func ParseGGUFMetadata(data []byte) (map[string]string, error) {
	return gguf.ParseMetadata(data)
}
