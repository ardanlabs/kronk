package models

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

// gguf file format identifiers.
const (
	ggufMagic   = 0x46554747
	ggufVersion = 3
)

// gguf metadata value type identifiers.
const (
	ggufMetadataValueTypeUInt8   uint32 = 0
	ggufMetadataValueTypeInt8    uint32 = 1
	ggufMetadataValueTypeUInt16  uint32 = 2
	ggufMetadataValueTypeInt16   uint32 = 3
	ggufMetadataValueTypeUInt32  uint32 = 4
	ggufMetadataValueTypeInt32   uint32 = 5
	ggufMetadataValueTypeFloat32 uint32 = 6
	ggufMetadataValueTypeBool    uint32 = 7
	ggufMetadataValueTypeString  uint32 = 8
	ggufMetadataValueTypeArray   uint32 = 9
	ggufMetadataValueTypeUInt64  uint32 = 10
	ggufMetadataValueTypeInt64   uint32 = 11
	ggufMetadataValueTypeFloat64 uint32 = 12
)

// =============================================================================

type ggufHeader struct {
	Magic           uint32
	Version         uint32
	TensorCount     uint64
	MetadataKvCount uint64
}

// ModelInfo represents the model's card information.
type ModelInfo struct {
	ID            string
	HasProjection bool
	Desc          string
	Size          uint64
	IsGPTModel    bool
	IsEmbedModel  bool
	IsRerankModel bool
	Metadata      map[string]string
}

// =============================================================================

// ModelInformation reads a GGUF model file and extracts model information.
func (m *Models) ModelInformation(modelID string) (ModelInfo, error) {
	modelID, _, _ = strings.Cut(modelID, "/")

	path, err := m.FullPath(modelID)
	if err != nil {
		return ModelInfo{}, fmt.Errorf("failed to retrieve path modelID[%s] file: %w", modelID, err)
	}

	var totalSize uint64
	for _, mf := range path.ModelFiles {
		info, err := os.Stat(mf)
		if err != nil {
			return ModelInfo{}, fmt.Errorf("failed to stat file: %w", err)
		}
		totalSize += uint64(info.Size())
	}

	file, err := os.Open(path.ModelFiles[0])
	if err != nil {
		return ModelInfo{}, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var header ggufHeader
	if err := binary.Read(file, binary.LittleEndian, &header.Magic); err != nil {
		return ModelInfo{}, fmt.Errorf("failed to read magic: %w", err)
	}

	if header.Magic != ggufMagic {
		return ModelInfo{}, fmt.Errorf("invalid GGUF magic number")
	}

	if err := binary.Read(file, binary.LittleEndian, &header.Version); err != nil {
		return ModelInfo{}, fmt.Errorf("failed to read version: %w", err)
	}

	if header.Version != ggufVersion {
		fmt.Printf("Warning: GGUF version %d is not supported (expected 3)\n", header.Version)
	}

	if err := binary.Read(file, binary.LittleEndian, &header.TensorCount); err != nil {
		return ModelInfo{}, fmt.Errorf("failed to read tensor count: %w", err)
	}

	if err := binary.Read(file, binary.LittleEndian, &header.MetadataKvCount); err != nil {
		return ModelInfo{}, fmt.Errorf("failed to read metadata count: %w", err)
	}

	metadata := make(map[string]string)
	for i := uint64(0); i < header.MetadataKvCount; i++ {
		key, value, err := readMetadataKV(file)
		if err != nil {
			fmt.Printf("Warning: failed to read metadata key-value pair %d: %v\n", i, err)
			continue
		}
		metadata[key] = fmt.Sprintf("%v", value)
	}

	var isGPTModel bool
	if strings.Contains(modelID, "gpt") {
		isGPTModel = true
	}

	var isEmbedModel bool
	if strings.Contains(modelID, "embed") {
		isEmbedModel = true
	}

	var isRerankModel bool
	if strings.Contains(modelID, "rerank") {
		isRerankModel = true
	}

	mi := ModelInfo{
		ID:            modelID,
		HasProjection: path.ProjFile != "",
		Desc:          metadata["general.name"],
		Size:          totalSize,
		IsGPTModel:    isGPTModel,
		IsEmbedModel:  isEmbedModel,
		IsRerankModel: isRerankModel,
		Metadata:      metadata,
	}

	return mi, nil
}

func readMetadataKV(file *os.File) (string, any, error) {
	var keyLen uint64
	if err := binary.Read(file, binary.LittleEndian, &keyLen); err != nil {
		return "", nil, err
	}

	if keyLen > 1*1024*1024 {
		return "", nil, fmt.Errorf("key length too large: %d", keyLen)
	}

	keyBytes := make([]byte, keyLen)
	if _, err := io.ReadFull(file, keyBytes); err != nil {
		return "", nil, err
	}
	key := string(keyBytes)

	var valueType uint32
	if err := binary.Read(file, binary.LittleEndian, &valueType); err != nil {
		return "", nil, err
	}

	value, err := readMetadataValue(file, valueType)
	if err != nil {
		// If we can't read the value due to unsupported type,
		// we still want to return the key but with an error
		// This way we don't break the entire parsing
		return key, nil, err
	}

	return key, value, nil
}

func readMetadataValue(file *os.File, valueType uint32) (interface{}, error) {
	switch valueType {
	case ggufMetadataValueTypeUInt8:
		var val uint8
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeInt8:
		var val int8
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeUInt16:
		var val uint16
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeInt16:
		var val int16
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeUInt32:
		var val uint32
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeInt32:
		var val int32
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeFloat32:
		var val float32
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeBool:
		var val uint8
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val != 0, nil

	case ggufMetadataValueTypeString:
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

	case ggufMetadataValueTypeArray:
		var arrayType uint32
		if err := binary.Read(file, binary.LittleEndian, &arrayType); err != nil {
			return nil, err
		}

		var arrayLen uint64
		if err := binary.Read(file, binary.LittleEndian, &arrayLen); err != nil {
			return nil, err
		}

		result := make([]any, arrayLen)
		for i := uint64(0); i < arrayLen; i++ {
			val, err := readMetadataValue(file, arrayType)
			if err != nil {
				return nil, err
			}
			result[i] = val
		}
		return result, nil

	case ggufMetadataValueTypeUInt64:
		var val uint64
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeInt64:
		var val int64
		if err := binary.Read(file, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case ggufMetadataValueTypeFloat64:
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
