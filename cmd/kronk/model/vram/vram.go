package vram

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	GGUF_MAGIC     = 0x46554747
	GGUF_VERSION_3 = 3
)

type GGUFMetadataValueType uint32

const (
	GGUF_METADATA_VALUE_TYPE_UINT8   GGUFMetadataValueType = 0
	GGUF_METADATA_VALUE_TYPE_INT8    GGUFMetadataValueType = 1
	GGUF_METADATA_VALUE_TYPE_UINT16  GGUFMetadataValueType = 2
	GGUF_METADATA_VALUE_TYPE_INT16   GGUFMetadataValueType = 3
	GGUF_METADATA_VALUE_TYPE_UINT32  GGUFMetadataValueType = 4
	GGUF_METADATA_VALUE_TYPE_INT32   GGUFMetadataValueType = 5
	GGUF_METADATA_VALUE_TYPE_FLOAT32 GGUFMetadataValueType = 6
	GGUF_METADATA_VALUE_TYPE_BOOL    GGUFMetadataValueType = 7
	GGUF_METADATA_VALUE_TYPE_STRING  GGUFMetadataValueType = 8
	GGUF_METADATA_VALUE_TYPE_ARRAY   GGUFMetadataValueType = 9
	GGUF_METADATA_VALUE_TYPE_UINT64  GGUFMetadataValueType = 10
	GGUF_METADATA_VALUE_TYPE_INT64   GGUFMetadataValueType = 11
	GGUF_METADATA_VALUE_TYPE_FLOAT64 GGUFMetadataValueType = 12
)

type GGUFMetadataValue struct {
	Type  GGUFMetadataValueType
	Value interface{}
}

type GGUFHeader struct {
	Magic           uint32
	Version         uint32
	TensorCount     uint64
	MetadataKvCount uint64
}

type GGUFMetadataKV struct {
	Key   string
	Value GGUFMetadataValue
}

type GGUFTensorInfo struct {
	Name       string
	NDim       uint32
	Dimensions []uint64
	Type       GGUFMetadataValueType
	Offset     uint64
}

type ModelArchitecture string

const (
	ARCH_LLAMA   ModelArchitecture = "llama"
	ARCH_MPT     ModelArchitecture = "mpt"
	ARCH_GPTNEOX ModelArchitecture = "gptneox"
	ARCH_GPTJ    ModelArchitecture = "gptj"
	ARCH_GPT2    ModelArchitecture = "gpt2"
	ARCH_BLOOM   ModelArchitecture = "bloom"
	ARCH_FALCON  ModelArchitecture = "falcon"
	ARCH_MAMBA   ModelArchitecture = "mamba"
	ARCH_RWKV    ModelArchitecture = "rwkv"
)

type ModelMetadata struct {
	Architecture        ModelArchitecture
	QuantizationVersion uint32
	Alignment           uint32
	Name                string
	Author              string
	Version             string
	Organization        string
	BaseName            string
	Finetune            string
	SizeLabel           string
	ContextLength       uint64
	EmbeddingLength     uint64
	BlockCount          uint64
	FeedForwardLength   uint64
	HeadCount           uint64
	HeadCountKV         uint64
	ExpertCount         uint32
	ExpertUsedCount     uint32
	RopeDimensionCount  uint64
	QuantizedBy         string
	FileType            uint32
	FileSize            uint64
}

var llamaLikeModelArch = []string{
	"qwen", "qwen2", "qwen3", "qwen3moe", "mistral", "starcoder", "codellama",
}

func runWeb(args []string) error {
	return fmt.Errorf("not implemented as we don't have access to the model metadata")
}

func runLocal(models *models.Models, modelID string, contextLength uint64, kvCacheType string) error {
	mi, err := models.RetrieveInfo(modelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model info: %w", err)
	}

	mp, err := models.RetrievePath(modelID)
	if err != nil {
		return fmt.Errorf("unable to retrieve model path: %w", err)
	}

	if len(mp.ModelFiles) == 0 {
		return fmt.Errorf("no model files found")
	}

	// Calculate the total size of all model files
	var totalSize int64
	for _, file := range mp.ModelFiles {
		info, err := os.Stat(file)
		if err != nil {
			return fmt.Errorf("unable to stat model file: %w", err)
		}
		totalSize += info.Size()
	}

	if err := kronk.Init(); err != nil {
		return fmt.Errorf("unable to init kronk: %w", err)
	}

	modelFilePath := mp.ModelFiles[0]

	modelMetadata, err := ParseGGUFFile(modelFilePath)
	if err != nil {
		return fmt.Errorf("unable to parse GGUF file: %w", err)
	}

	vramUsage := CalculateVRAM(modelMetadata, contextLength, kvCacheType)

	fmt.Println("=== Model Information ===")
	fmt.Printf("ID: %s\n", mi.ID)
	fmt.Printf("Owned By: %s\n", mi.OwnedBy)
	fmt.Printf("Size: %s\n", FormatBytes(uint64(totalSize)))
	fmt.Printf("Architecture: %s\n", modelMetadata.Architecture)
	fmt.Printf("Name: %s\n", modelMetadata.Name)
	fmt.Printf("Context Length: %d\n", modelMetadata.ContextLength)
	fmt.Printf("Embedding Length: %d\n", modelMetadata.EmbeddingLength)
	fmt.Printf("Block Count: %d\n", modelMetadata.BlockCount)
	fmt.Printf("Head Count: %d\n", modelMetadata.HeadCount)
	fmt.Printf("Head Count KV: %d\n", modelMetadata.HeadCountKV)
	fmt.Printf("Head Count KV: %d\n", modelMetadata.HeadCountKV)
	fmt.Printf("Expert Count: %d\n", modelMetadata.ExpertCount)
	fmt.Printf("Expert Used Count: %d\n", modelMetadata.ExpertUsedCount)
	fmt.Printf("RoPE Dimension Count: %d\n", modelMetadata.RopeDimensionCount)
	if modelMetadata.SizeLabel != "" {
		expertCount, paramCount, activeParamCount, err := ParseSizeLabel(modelMetadata.SizeLabel)
		if err == nil {
			fmt.Printf("Parsed Size - Expert Count: %d, Parameter Count: %s, Active Parameter Count: %s\n", expertCount, paramCount, activeParamCount)
		}
	}

	fmt.Println("\n=== VRAM Calculation ===")
	fmt.Printf("KV Cache Type: %s\n", kvCacheType)
	fmt.Printf("Estimated VRAM Usage: %s\n", FormatBytes(vramUsage))
	fmt.Printf("Context Length: %d tokens\n", contextLength)
	if contextLength > modelMetadata.ContextLength {
		fmt.Printf("Warning: Context length %d exceeds model's maximum context length %d\n", contextLength, modelMetadata.ContextLength)

		atMaxModelContextSize := FormatBytes(CalculateVRAM(modelMetadata, modelMetadata.ContextLength, kvCacheType))
		fmt.Printf("VRAM Usage at Maximum Model Context Length: %s\n", atMaxModelContextSize)
	}

	fmt.Println("\n=== VRAM Usage for Common Context Lengths ===")
	commonContexts := []uint64{512, 768, 1024, 2048, 4096, 8192, 16384, 32768, 65536, 98304, 131072}
	for _, ctxSize := range commonContexts {
		vram := CalculateVRAM(modelMetadata, ctxSize, kvCacheType)
		fmt.Printf("Context %d: %s\n", ctxSize, FormatBytes(vram))
	}

	return nil
}

var kvCacheBytesPerElement = map[string]float64{
	"fp32":    4.0,
	"fp16":    2.0,
	"bf16":    2.0,
	"fp8":     1.0,
	"q8_0":    1.0,
	"q8":      1.0,
	"q6_k":    0.75,
	"q5_1":    0.6875,
	"q5_0":    0.625,
	"q5_k":    0.625,
	"q4_1":    0.5625,
	"q4_0":    0.5,
	"q4_k":    0.5,
	"q4":      0.5,
	"q3_k":    0.4375,
	"q2_k":    0.3125,
	"iq4_nl":  0.5,
	"iq4_xs":  0.5,
	"iq3_xxs": 0.375,
	"iq3_s":   0.375,
	"iq2_xxs": 0.25,
	"iq2_xs":  0.25,
	"iq2_s":   0.25,
	"iq1_s":   0.1875,
	"iq1_m":   0.1875,
}

func CalculateVRAM(metadata ModelMetadata, contextLength uint64, kvCacheType string) uint64 {
	alignment := metadata.Alignment
	if alignment == 0 {
		alignment = 32
	}

	arch := metadata.Architecture

	switch arch {
	case ARCH_LLAMA:
		return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
	case ARCH_MPT:
		return calculateMPTVRAM(metadata, contextLength, alignment, kvCacheType)
	case ARCH_GPTNEOX:
		return calculateGPTNeoXVRAM(metadata, contextLength, alignment, kvCacheType)
	case ARCH_GPTJ:
		return calculateGPTJVRAM(metadata, contextLength, alignment, kvCacheType)
	case ARCH_GPT2:
		return calculateGPT2VRAM(metadata, contextLength, alignment, kvCacheType)
	case ARCH_BLOOM:
		return calculateBLOOMVRAM(metadata, contextLength, alignment, kvCacheType)
	case ARCH_FALCON:
		return calculateFALCONVRAM(metadata, contextLength, alignment, kvCacheType)
	case ARCH_MAMBA:
		return calculateMAMBAVRAM(metadata, contextLength, alignment)
	case ARCH_RWKV:
		return calculateRWKVVRAM(metadata, contextLength, alignment)
	default:
		archStr := string(arch)
		for _, prefix := range llamaLikeModelArch {
			if strings.HasPrefix(archStr, prefix) {
				return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
			}
		}

		return calculateDefaultVRAM(metadata, contextLength, alignment, kvCacheType)
	}
}

func calculateLlamaVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	nLayers := metadata.BlockCount
	nHeads := metadata.HeadCount
	nHeadsKV := metadata.HeadCountKV
	embd := metadata.EmbeddingLength

	if nHeadsKV == 0 {
		nHeadsKV = nHeads
	}

	var dHead uint64 = 128
	if nHeads > 0 && embd > 0 {
		dHead = embd / nHeads
	}

	modelWeightsVRAM := metadata.FileSize

	bytesPerElement, ok := kvCacheBytesPerElement[strings.ToLower(kvCacheType)]
	if !ok {
		bytesPerElement = 2.0 // default to FP16
	}

	// KV cache size: 2 (K and V) * nLayers * nHeadsKV * dHead * contextLength * bytesPerElement
	kvCacheBytes := uint64(float64(2*nLayers*nHeadsKV*dHead*contextLength) * bytesPerElement)

	overhead := modelWeightsVRAM / 10

	totalVRAM := modelWeightsVRAM + kvCacheBytes + overhead

	return alignVRAM(totalVRAM, uint64(alignment))
}

func calculateMPTVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateGPTNeoXVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateGPTJVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateGPT2VRAM(metadata ModelMetadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateBLOOMVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateFALCONVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func calculateMAMBAVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32) uint64 {
	// Mamba doesn't have traditional KV cache, just state
	modelWeightsVRAM := metadata.FileSize
	overhead := modelWeightsVRAM / 10

	// Mamba state is much smaller than KV cache
	nLayers := metadata.BlockCount
	stateSize := metadata.FeedForwardLength
	if stateSize == 0 {
		stateSize = 16
	}
	stateBytes := nLayers * stateSize * 2

	return alignVRAM(modelWeightsVRAM+stateBytes+overhead, uint64(alignment))
}

func calculateRWKVVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32) uint64 {
	modelWeightsVRAM := metadata.FileSize
	overhead := modelWeightsVRAM / 10

	// RWKV has fixed state regardless of context length
	nLayers := metadata.BlockCount
	embd := metadata.EmbeddingLength
	stateBytes := nLayers * embd * 5 * 2

	return alignVRAM(modelWeightsVRAM+stateBytes+overhead, uint64(alignment))
}

// calculateDefaultVRAM calculates VRAM for unknown architectures
func calculateDefaultVRAM(metadata ModelMetadata, contextLength uint64, alignment uint32, kvCacheType string) uint64 {
	return calculateLlamaVRAM(metadata, contextLength, alignment, kvCacheType)
}

func getBytesPerWeight(metadata ModelMetadata) float64 {
	fileTypeBytes := map[uint32]float64{
		0:  4.0,    // ALL_F32
		1:  2.0,    // MOSTLY_F16
		2:  0.5,    // MOSTLY_Q4_0
		3:  0.5625, // MOSTLY_Q4_1
		7:  1.0,    // MOSTLY_Q8_0
		8:  0.625,  // MOSTLY_Q5_0
		9:  0.6875, // MOSTLY_Q5_1
		10: 0.3125, // MOSTLY_Q2_K
		11: 0.375,  // MOSTLY_Q3_K_S
		12: 0.4375, // MOSTLY_Q3_K_M
		13: 0.5,    // MOSTLY_Q3_K_L
		14: 0.5,    // MOSTLY_Q4_K_S
		15: 0.5625, // MOSTLY_Q4_K_M
		16: 0.625,  // MOSTLY_Q5_K_S
		17: 0.6875, // MOSTLY_Q5_K_M
		18: 0.8125, // MOSTLY_Q6_K
	}

	if bpw, exists := fileTypeBytes[metadata.FileType]; exists {
		return bpw
	}

	return 2.0
}

// alignVRAM aligns VRAM usage to the specified alignment
func alignVRAM(vram, alignment uint64) uint64 {
	if alignment == 0 {
		alignment = 32
	}
	return ((vram + alignment - 1) / alignment) * alignment
}

func ParseGGUFFile(filename string) (ModelMetadata, error) {
	file, err := os.Open(filename)
	if err != nil {
		return ModelMetadata{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return ModelMetadata{}, fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := uint64(fileInfo.Size())

	var header GGUFHeader
	if err := binary.Read(file, binary.LittleEndian, &header.Magic); err != nil {
		return ModelMetadata{}, fmt.Errorf("failed to read magic: %w", err)
	}

	if header.Magic != GGUF_MAGIC {
		return ModelMetadata{}, fmt.Errorf("invalid GGUF magic number")
	}

	if err := binary.Read(file, binary.LittleEndian, &header.Version); err != nil {
		return ModelMetadata{}, fmt.Errorf("failed to read version: %w", err)
	}

	if header.Version != GGUF_VERSION_3 {
		fmt.Printf("Warning: GGUF version %d is not supported (expected 3)\n", header.Version)
	}

	if err := binary.Read(file, binary.LittleEndian, &header.TensorCount); err != nil {
		return ModelMetadata{}, fmt.Errorf("failed to read tensor count: %w", err)
	}

	if err := binary.Read(file, binary.LittleEndian, &header.MetadataKvCount); err != nil {
		return ModelMetadata{}, fmt.Errorf("failed to read metadata count: %w", err)
	}

	metadata := make(map[string]GGUFMetadataValue)

	for i := uint64(0); i < header.MetadataKvCount; i++ {
		key, value, err := readMetadataKV(file)
		if err != nil {
			fmt.Printf("Warning: failed to read metadata key-value pair %d: %v\n", i, err)
			continue
		}
		metadata[key] = value
	}

	modelMetadata := ModelMetadata{
		Alignment: 32,
		FileSize:  fileSize,
	}

	if val, exists := metadata["general.architecture"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.Architecture = ModelArchitecture(str)
		}
	}

	if val, exists := metadata["general.quantization_version"]; exists {
		if uint32Val, ok := val.Value.(uint32); ok {
			modelMetadata.QuantizationVersion = uint32Val
		}
	}

	if val, exists := metadata["general.alignment"]; exists {
		if uint32Val, ok := val.Value.(uint32); ok {
			modelMetadata.Alignment = uint32Val
		}
	}

	if val, exists := metadata["general.name"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.Name = str
		}
	}

	if val, exists := metadata["general.author"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.Author = str
		}
	}

	if val, exists := metadata["general.version"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.Version = str
		}
	}

	if val, exists := metadata["general.organization"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.Organization = str
		}
	}

	if val, exists := metadata["general.basename"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.BaseName = str
		}
	}

	if val, exists := metadata["general.finetune"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.Finetune = str
		}
	}

	if val, exists := metadata["general.size_label"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.SizeLabel = str
		}
	}

	if val, exists := metadata["general.quantized_by"]; exists {
		if str, ok := val.Value.(string); ok {
			modelMetadata.QuantizedBy = str
		}
	}

	if val, exists := metadata["general.file_type"]; exists {
		switch v := val.Value.(type) {
		case uint32:
			modelMetadata.FileType = v
		case uint64:
			modelMetadata.FileType = uint32(v)
		case int32:
			modelMetadata.FileType = uint32(v)
		}
	}

	arch := string(modelMetadata.Architecture)
	if arch == "" {
		arch = "llama"
	}

	getUint64 := func(key string) uint64 {
		if val, exists := metadata[key]; exists {
			switch v := val.Value.(type) {
			case uint64:
				return v
			case uint32:
				return uint64(v)
			case int64:
				return uint64(v)
			case int32:
				return uint64(v)
			}
		}
		return 0
	}
	getUint32 := func(key string) uint32 {
		if val, exists := metadata[key]; exists {
			switch v := val.Value.(type) {
			case uint32:
				return v
			case uint64:
				return uint32(v)
			case int32:
				return uint32(v)
			case int64:
				return uint32(v)
			}
		}
		return 0
	}

	modelMetadata.ContextLength = getUint64(arch + ".context_length")
	modelMetadata.EmbeddingLength = getUint64(arch + ".embedding_length")
	modelMetadata.BlockCount = getUint64(arch + ".block_count")
	modelMetadata.FeedForwardLength = getUint64(arch + ".feed_forward_length")
	modelMetadata.HeadCount = getUint64(arch + ".attention.head_count")
	modelMetadata.HeadCountKV = getUint64(arch + ".attention.head_count_kv")
	modelMetadata.ExpertCount = getUint32(arch + ".expert_count")
	modelMetadata.ExpertUsedCount = getUint32(arch + ".expert_used_count")
	modelMetadata.RopeDimensionCount = getUint64(arch + ".rope.dimension_count")

	return modelMetadata, nil
}

func readMetadataKV(file *os.File) (string, GGUFMetadataValue, error) {
	var keyLen uint64
	if err := binary.Read(file, binary.LittleEndian, &keyLen); err != nil {
		return "", GGUFMetadataValue{}, err
	}

	if keyLen > 1*1024*1024 {
		return "", GGUFMetadataValue{}, fmt.Errorf("key length too large: %d", keyLen)
	}

	keyBytes := make([]byte, keyLen)
	if _, err := io.ReadFull(file, keyBytes); err != nil {
		return "", GGUFMetadataValue{}, err
	}
	key := string(keyBytes)

	var valueType GGUFMetadataValueType
	if err := binary.Read(file, binary.LittleEndian, &valueType); err != nil {
		return "", GGUFMetadataValue{}, err
	}

	value, err := readMetadataValue(file, valueType)
	if err != nil {
		// If we can't read the value due to unsupported type,
		// we still want to return the key but with an error
		// This way we don't break the entire parsing
		return key, GGUFMetadataValue{Type: valueType}, err
	}

	return key, GGUFMetadataValue{Type: valueType, Value: value}, nil
}

func readMetadataValue(file *os.File, valueType GGUFMetadataValueType) (interface{}, error) {
	switch valueType {
	case GGUF_METADATA_VALUE_TYPE_UINT8:
		var val uint8
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_INT8:
		var val int8
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_UINT16:
		var val uint16
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_INT16:
		var val int16
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_UINT32:
		var val uint32
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_INT32:
		var val int32
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_FLOAT32:
		var val float32
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_BOOL:
		var val uint8
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val != 0, nil
	case GGUF_METADATA_VALUE_TYPE_STRING:
		var strLen uint64
		if err := binary.Read(file, binary.LittleEndian, &strLen); err != nil {
			return nil, err
		}

		if strLen > 1*1024*1024 {
			return nil, fmt.Errorf("string length too large: %d", strLen)
		}

		strBytes := make([]byte, strLen)
		if _, err := io.ReadFull(file, strBytes); err != nil {
			return nil, err
		}

		return string(strBytes), nil
	case GGUF_METADATA_VALUE_TYPE_ARRAY:
		var arrayType GGUFMetadataValueType
		if err := binary.Read(file, binary.LittleEndian, &arrayType); err != nil {
			return nil, err
		}

		var arrayLen uint64
		if err := binary.Read(file, binary.LittleEndian, &arrayLen); err != nil {
			return nil, err
		}

		for i := uint64(0); i < arrayLen; i++ {
			if _, err := readMetadataValue(file, arrayType); err != nil {
				return nil, err
			}
		}

		return nil, nil
	case GGUF_METADATA_VALUE_TYPE_UINT64:
		var val uint64
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_INT64:
		var val int64
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	case GGUF_METADATA_VALUE_TYPE_FLOAT64:
		var val float64
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}

		return val, nil
	default:
		// For unsupported metadata value types, we return an error
		// This prevents any data reading that could cause panic
		return nil, fmt.Errorf("unsupported metadata value type: %d", valueType)
	}
}

func FormatBytes(bytes uint64) string {
	if bytes < 1024 {
		return fmt.Sprintf("%d B", bytes)
	}
	if bytes < 1024*1024 {
		return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
	}
	if bytes < 1024*1024*1024 {
		return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
	}
	if bytes < 1024*1024*1024*1024 {
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	}
	return fmt.Sprintf("%.2f TB", float64(bytes)/(1024*1024*1024*1024))
}

func ParseSizeLabel(sizeLabel string) (expertCount uint32, parameterCount, activeParamCount string, err error) {
	switch {
	// Handle the expert count format like "8x7B"
	case strings.Contains(sizeLabel, "x"):
		parts := strings.SplitN(sizeLabel, "x", 2)
		if len(parts) < 2 {
			return 0, sizeLabel, "-", err
		}

		// Parse expert count
		if expertCountVal, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
			expertCount = uint32(expertCountVal)
		}

		parameterPart := strings.Join(parts[1:], "x")
		return expertCount, parameterPart, activeParamCount, err

	// Handle the expert count format like "30B-A3B"
	case strings.Contains(sizeLabel, "-A"):
		parts := strings.SplitN(sizeLabel, "-A", 2)
		if len(parts) < 2 {
			return 0, sizeLabel, "-", err
		}

		// Parse expert count
		if expertCountVal, err := strconv.ParseUint(parts[0], 10, 32); err == nil {
			expertCount = uint32(expertCountVal)
		}

		parameterPart := parts[0]
		activeParamCount = parts[1]
		return expertCount, parameterPart, activeParamCount, err

	default:
		return 0, sizeLabel, "-", err
	}
}
