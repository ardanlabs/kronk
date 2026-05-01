package gguf

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// TensorInfo holds parsed tensor descriptor information from a GGUF file.
type TensorInfo struct {
	Name     string
	NDims    uint32
	Dims     []int64
	GGMLType uint32
	Offset   uint64
	Bytes    int64
}

// ggmlTypeInfo holds block size and type size for ggml quantization types.
type ggmlTypeInfo struct {
	blockSize int64
	typeSize  int64
}

// ggmlTypeSizes maps a ggml_type ID to its block size and per-block byte
// size. The numbers come straight from llama.cpp's ggml.c.
var ggmlTypeSizes = map[uint32]ggmlTypeInfo{
	0:  {1, 4},     // F32
	1:  {1, 2},     // F16
	2:  {32, 18},   // Q4_0
	3:  {32, 20},   // Q4_1
	6:  {32, 22},   // Q5_0
	7:  {32, 24},   // Q5_1
	8:  {32, 34},   // Q8_0
	9:  {32, 36},   // Q8_1
	10: {256, 82},  // Q2_K
	11: {256, 110}, // Q3_K
	12: {256, 144}, // Q4_K
	13: {256, 176}, // Q5_K
	14: {256, 210}, // Q6_K
	15: {256, 256}, // Q8_K
	16: {256, 54},  // IQ2_XXS
	17: {256, 66},  // IQ2_XS
	18: {256, 258}, // IQ3_XXS
	19: {256, 50},  // IQ1_S
	20: {32, 18},   // IQ4_NL
	21: {256, 110}, // IQ3_S
	22: {256, 82},  // IQ2_S
	23: {256, 82},  // IQ4_XS
	24: {1, 1},     // I8
	25: {1, 2},     // I16
	26: {1, 4},     // I32
	27: {1, 8},     // I64
	28: {1, 8},     // F64
	29: {256, 56},  // IQ1_M
	30: {1, 2},     // BF16
	34: {256, 54},  // TQ1_0
	35: {256, 66},  // TQ2_0
}

// GGMLRowSize computes the byte size of a single row of ne0 elements for
// the given ggml_type ID.
func GGMLRowSize(ggmlType uint32, ne0 int64) int64 {
	info, ok := ggmlTypeSizes[ggmlType]
	if !ok {
		return 0
	}

	nBlocks := (ne0 + info.blockSize - 1) / info.blockSize

	return nBlocks * info.typeSize
}

// GGMLTensorSize computes the total byte size of a tensor given its
// ggml_type ID and dimensions.
func GGMLTensorSize(ggmlType uint32, dims []int64) int64 {
	if len(dims) == 0 {
		return 0
	}

	rowBytes := GGMLRowSize(ggmlType, dims[0])
	total := rowBytes
	for i := 1; i < len(dims); i++ {
		total *= dims[i]
	}

	return total
}

// ParseHeaderAndTensors parses the GGUF magic, KV metadata, and tensor
// table from already-fetched header bytes (catalog cache or local file).
// fileSize is only used in error context.
func ParseHeaderAndTensors(data []byte, fileSize int64) (metadata map[string]string, tensors []TensorInfo, err error) {
	reader := bytes.NewReader(data)

	header, err := readHeader(reader)
	if err != nil {
		return nil, nil, fmt.Errorf("parse-header-tensors: data_len=%d, file_size=%d: %w", len(data), fileSize, err)
	}

	metadata = make(map[string]string, header.MetadataKvCount)
	var kvParseErr error
	var kvParsed uint64
	for i := uint64(0); i < header.MetadataKvCount; i++ {
		key, value, kvErr := readMetadataKV(reader)
		if kvErr != nil {
			kvParseErr = kvErr
			break
		}
		kvParsed++
		metadata[key] = fmt.Sprintf("%v", value)
	}

	bytesAfterKV := reader.Len()

	tensors, err = parseTensorTable(reader, header.TensorCount)
	if err != nil {
		return nil, nil, fmt.Errorf("parse-header-tensors: failed to parse tensor table: data_len=%d, file_size=%d, version=%d, tensors=%d, kv_count=%d, kv_parsed=%d, kv_err=%v, bytes_remaining_after_kv=%d: %w",
			len(data), fileSize, header.Version, header.TensorCount, header.MetadataKvCount, kvParsed, kvParseErr, bytesAfterKV, err)
	}

	return metadata, tensors, nil
}

// parseTensorTable reads tensor descriptors from a bytes.Reader positioned
// at the start of the tensor table.
func parseTensorTable(r *bytes.Reader, tensorCount uint64) ([]TensorInfo, error) {
	tensors := make([]TensorInfo, 0, tensorCount)

	for i := range tensorCount {
		var nameLen uint64
		if err := binary.Read(r, binary.LittleEndian, &nameLen); err != nil {
			return nil, fmt.Errorf("parse-tensor-table: reading name length for tensor %d: %w", i, err)
		}

		if nameLen > 1*1024*1024 {
			return nil, fmt.Errorf("parse-tensor-table: name length too large: %d", nameLen)
		}

		nameBytes := make([]byte, nameLen)
		if _, err := io.ReadFull(r, nameBytes); err != nil {
			return nil, fmt.Errorf("parse-tensor-table: reading name for tensor %d: %w", i, err)
		}

		var nDims uint32
		if err := binary.Read(r, binary.LittleEndian, &nDims); err != nil {
			return nil, fmt.Errorf("parse-tensor-table: reading n_dims for tensor %d: %w", i, err)
		}

		dims := make([]int64, nDims)
		for d := uint32(0); d < nDims; d++ {
			var dim uint64
			if err := binary.Read(r, binary.LittleEndian, &dim); err != nil {
				return nil, fmt.Errorf("parse-tensor-table: reading dim %d for tensor %d: %w", d, i, err)
			}
			dims[d] = int64(dim)
		}

		var ggmlType uint32
		if err := binary.Read(r, binary.LittleEndian, &ggmlType); err != nil {
			return nil, fmt.Errorf("parse-tensor-table: reading ggml_type for tensor %d: %w", i, err)
		}

		var offset uint64
		if err := binary.Read(r, binary.LittleEndian, &offset); err != nil {
			return nil, fmt.Errorf("parse-tensor-table: reading offset for tensor %d: %w", i, err)
		}

		tensorBytes := GGMLTensorSize(ggmlType, dims)

		tensors = append(tensors, TensorInfo{
			Name:     string(nameBytes),
			NDims:    nDims,
			Dims:     dims,
			GGMLType: ggmlType,
			Offset:   offset,
			Bytes:    tensorBytes,
		})
	}

	return tensors, nil
}
