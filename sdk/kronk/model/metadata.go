package model

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
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

type Metadata struct {
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

func ParseGGUFFile(filename string) (Metadata, error) {
	file, err := os.Open(filename)
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		return Metadata{}, fmt.Errorf("failed to stat file: %w", err)
	}
	fileSize := uint64(fileInfo.Size())

	var header GGUFHeader
	if err := binary.Read(file, binary.LittleEndian, &header.Magic); err != nil {
		return Metadata{}, fmt.Errorf("failed to read magic: %w", err)
	}

	if header.Magic != GGUF_MAGIC {
		return Metadata{}, fmt.Errorf("invalid GGUF magic number")
	}

	if err := binary.Read(file, binary.LittleEndian, &header.Version); err != nil {
		return Metadata{}, fmt.Errorf("failed to read version: %w", err)
	}

	if header.Version != GGUF_VERSION_3 {
		fmt.Printf("Warning: GGUF version %d is not supported (expected 3)\n", header.Version)
	}

	if err := binary.Read(file, binary.LittleEndian, &header.TensorCount); err != nil {
		return Metadata{}, fmt.Errorf("failed to read tensor count: %w", err)
	}

	if err := binary.Read(file, binary.LittleEndian, &header.MetadataKvCount); err != nil {
		return Metadata{}, fmt.Errorf("failed to read metadata count: %w", err)
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

	modelMetadata := Metadata{
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
