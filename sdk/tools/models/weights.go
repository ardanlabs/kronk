package models

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// WeightBreakdown provides per-category weight size information.
type WeightBreakdown struct {
	TotalBytes         int64
	AlwaysActiveBytes  int64
	ExpertBytesTotal   int64
	ExpertBytesByLayer []int64
}

// ggufTensorInfo holds parsed tensor descriptor information from a GGUF file.
type ggufTensorInfo struct {
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

// Broad pattern for VRAM accounting: catches all routed expert tensors.
// Covers standard (_exps) and channel (_chexps) expert variants as defined
// in llama.cpp and yzma's MoEExpertTensorPattern. The blk.<n>. prefix is
// required for per-layer attribution in ExpertBytesByLayer. The suffix
// boundary (\. or end) prevents accidental matches inside longer names.
var expertTensorPattern = regexp.MustCompile(`blk\.\d+\.ffn_(up|down|gate|gate_up|norm)_(ch)?exps(\.|$)`)

// blockIndexPattern extracts the block index from a tensor name.
var blockIndexPattern = regexp.MustCompile(`^blk\.(\d+)\.`)

// ggmlRowSize computes the byte size of a single row of ne0 elements.
func ggmlRowSize(ggmlType uint32, ne0 int64) int64 {
	info, ok := ggmlTypeSizes[ggmlType]
	if !ok {
		return 0
	}

	nBlocks := (ne0 + info.blockSize - 1) / info.blockSize

	return nBlocks * info.typeSize
}

// ggmlTensorSize computes the total byte size of a tensor.
func ggmlTensorSize(ggmlType uint32, dims []int64) int64 {
	if len(dims) == 0 {
		return 0
	}

	rowBytes := ggmlRowSize(ggmlType, dims[0])
	total := rowBytes
	for i := 1; i < len(dims); i++ {
		total *= dims[i]
	}

	return total
}

// categorizeWeights builds a WeightBreakdown from parsed tensor info.
// blockCount is the number of transformer layers from metadata.
func categorizeWeights(tensors []ggufTensorInfo, blockCount int64) WeightBreakdown {
	wb := WeightBreakdown{
		ExpertBytesByLayer: make([]int64, blockCount),
	}

	for _, t := range tensors {
		if expertTensorPattern.MatchString(t.Name) {
			wb.ExpertBytesTotal += t.Bytes

			if m := blockIndexPattern.FindStringSubmatch(t.Name); len(m) == 2 {
				idx, err := strconv.ParseInt(m[1], 10, 64)
				if err == nil && idx >= 0 && idx < blockCount {
					wb.ExpertBytesByLayer[idx] += t.Bytes
				}
			}

			continue
		}

		wb.AlwaysActiveBytes += t.Bytes
	}

	wb.TotalBytes = wb.AlwaysActiveBytes + wb.ExpertBytesTotal

	return wb
}

// detectMoEFromTensors returns true if tensor names contain expert patterns,
// providing a fallback when GGUF metadata keys are missing.
func detectMoEFromTensors(tensors []ggufTensorInfo) bool {
	for _, t := range tensors {
		if expertTensorPattern.MatchString(t.Name) {
			return true
		}
	}

	return false
}

// detectSharedExpertsFromTensors checks for shared expert tensors in names.
func detectSharedExpertsFromTensors(tensors []ggufTensorInfo) bool {
	for _, t := range tensors {
		lower := strings.ToLower(t.Name)
		if strings.Contains(lower, "shared") || strings.Contains(lower, "shexp") {
			return true
		}
	}

	return false
}

// ggufHeaderFetchSize is the number of bytes fetched in a single Range
// request to cover the fixed header, all KV metadata, and all tensor
// descriptors for any model. 16 MiB covers even MoE models whose
// metadata embeds large per-layer arrays.
//
// Do NOT reduce this value. 8 MiB was tested and is too small for
// large MoE models whose metadata exceeds that threshold.
const ggufHeaderFetchSize = 16 * 1024 * 1024

// fetchGGUFHeaderAndTensors fetches GGUF header, KV metadata, and tensor
// descriptors from a remote URL using HTTP Range requests. Only the header
// sections are downloaded, not the actual tensor data.
func fetchGGUFHeaderAndTensors(ctx context.Context, url string) (metadata map[string]string, tensors []ggufTensorInfo, fileSize int64, err error) {
	var client http.Client

	data, fileSize, err := fetchRange(ctx, &client, url, 0, ggufHeaderFetchSize-1)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("fetch-gguf-header-tensors: failed to fetch header data: %w", err)
	}

	reader := bytes.NewReader(data)

	var header ggufHeader
	if err := binary.Read(reader, binary.LittleEndian, &header.Magic); err != nil {
		return nil, nil, 0, fmt.Errorf("fetch-gguf-header-tensors: failed to read magic (data_len=%d): %w", len(data), err)
	}

	if header.Magic != ggufMagic {
		return nil, nil, 0, fmt.Errorf("fetch-gguf-header-tensors: invalid GGUF magic number: got 0x%X, first_16_bytes=%x, data_len=%d", header.Magic, data[:min(16, len(data))], len(data))
	}

	if err := binary.Read(reader, binary.LittleEndian, &header.Version); err != nil {
		return nil, nil, 0, fmt.Errorf("fetch-gguf-header-tensors: failed to read version: %w", err)
	}

	if err := binary.Read(reader, binary.LittleEndian, &header.TensorCount); err != nil {
		return nil, nil, 0, fmt.Errorf("fetch-gguf-header-tensors: failed to read tensor count: %w", err)
	}

	if err := binary.Read(reader, binary.LittleEndian, &header.MetadataKvCount); err != nil {
		return nil, nil, 0, fmt.Errorf("fetch-gguf-header-tensors: failed to read metadata count: %w", err)
	}

	// Parse KV metadata.
	metadata = make(map[string]string)
	var kvParseErr error
	var kvParsed uint64
	for i := uint64(0); i < header.MetadataKvCount; i++ {
		key, value, kvErr := readMetadataKVFromReader(reader)
		if kvErr != nil {
			kvParseErr = kvErr
			break
		}
		kvParsed++
		metadata[key] = fmt.Sprintf("%v", value)
	}

	bytesAfterKV := reader.Len()

	// Parse tensor descriptors from the remaining data.
	tensors, err = parseTensorTableFromReader(reader, header.TensorCount)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("fetch-gguf-header-tensors: failed to parse tensor table: data_len=%d, file_size=%d, version=%d, tensors=%d, kv_count=%d, kv_parsed=%d, kv_err=%v, bytes_remaining_after_kv=%d: %w",
			len(data), fileSize, header.Version, header.TensorCount, header.MetadataKvCount, kvParsed, kvParseErr, bytesAfterKV, err)
	}

	return metadata, tensors, fileSize, nil
}

// parseTensorTableFromReader reads tensor descriptors from a bytes.Reader.
func parseTensorTableFromReader(r *bytes.Reader, tensorCount uint64) ([]ggufTensorInfo, error) {
	tensors := make([]ggufTensorInfo, 0, tensorCount)

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

		tensorBytes := ggmlTensorSize(ggmlType, dims)

		tensors = append(tensors, ggufTensorInfo{
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
