package gguf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// ParseMetadata parses the GGUF header and key-value metadata from a byte
// slice (typically the first HeaderFetchSize bytes of a GGUF file). The
// caller is responsible for sourcing the bytes (local file, HTTP Range,
// catalog cache, etc).
//
// Values are stringified via fmt.Sprintf("%v", ...). Array-typed values
// surface as Go's default slice format ("[a b c]") which the
// ParseInt64OrArrayAvg helper understands.
func ParseMetadata(data []byte) (map[string]string, error) {
	reader := bytes.NewReader(data)

	header, err := readHeader(reader)
	if err != nil {
		return nil, fmt.Errorf("parse-metadata: %w", err)
	}

	metadata := make(map[string]string, header.MetadataKvCount)
	for i := uint64(0); i < header.MetadataKvCount; i++ {
		key, value, err := readMetadataKV(reader)
		if err != nil {
			break
		}
		metadata[key] = fmt.Sprintf("%v", value)
	}

	return metadata, nil
}

// readHeader reads the fixed-size GGUF header and validates the magic.
func readHeader(r *bytes.Reader) (Header, error) {
	var header Header

	if err := binary.Read(r, binary.LittleEndian, &header.Magic); err != nil {
		return Header{}, fmt.Errorf("read-header: read magic: %w", err)
	}

	if header.Magic != Magic {
		return Header{}, fmt.Errorf("read-header: invalid GGUF magic number: got 0x%X", header.Magic)
	}

	if err := binary.Read(r, binary.LittleEndian, &header.Version); err != nil {
		return Header{}, fmt.Errorf("read-header: read version: %w", err)
	}

	if err := binary.Read(r, binary.LittleEndian, &header.TensorCount); err != nil {
		return Header{}, fmt.Errorf("read-header: read tensor count: %w", err)
	}

	if err := binary.Read(r, binary.LittleEndian, &header.MetadataKvCount); err != nil {
		return Header{}, fmt.Errorf("read-header: read metadata count: %w", err)
	}

	return header, nil
}

// readMetadataKV reads a single key/value metadata pair from a reader
// positioned at the start of the pair.
func readMetadataKV(r *bytes.Reader) (string, any, error) {
	var keyLen uint64
	if err := binary.Read(r, binary.LittleEndian, &keyLen); err != nil {
		return "", nil, err
	}

	if keyLen > 1*1024*1024 {
		return "", nil, fmt.Errorf("read-metadata-kv: key length too large: %d", keyLen)
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

	value, err := readMetadataValue(r, valueType)
	if err != nil {
		return key, nil, err
	}

	return key, value, nil
}

// readMetadataValue reads a single metadata value from a reader given the
// value's type tag.
func readMetadataValue(r *bytes.Reader, valueType uint32) (any, error) {
	switch valueType {
	case MetadataValueTypeUInt8:
		var val uint8
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeInt8:
		var val int8
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeUInt16:
		var val uint16
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeInt16:
		var val int16
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeUInt32:
		var val uint32
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeInt32:
		var val int32
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeFloat32:
		var val float32
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeBool:
		var val uint8
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val != 0, nil

	case MetadataValueTypeString:
		var strLen uint64
		if err := binary.Read(r, binary.LittleEndian, &strLen); err != nil {
			return nil, err
		}

		if strLen > 1*1024*1024 {
			return nil, fmt.Errorf("read-metadata-value: string length too large: %d", strLen)
		}

		strBytes := make([]byte, strLen)
		if _, err := io.ReadFull(r, strBytes); err != nil {
			return nil, err
		}
		return string(strBytes), nil

	case MetadataValueTypeArray:
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
			val, err := readMetadataValue(r, arrayType)
			if err != nil {
				return nil, err
			}
			result[i] = val
		}
		return result, nil

	case MetadataValueTypeUInt64:
		var val uint64
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeInt64:
		var val int64
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	case MetadataValueTypeFloat64:
		var val float64
		if err := binary.Read(r, binary.LittleEndian, &val); err != nil {
			return nil, err
		}
		return val, nil

	default:
		return nil, fmt.Errorf("read-metadata-value: unsupported metadata value type: %d", valueType)
	}
}
