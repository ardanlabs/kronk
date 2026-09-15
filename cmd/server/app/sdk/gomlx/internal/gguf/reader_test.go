package gguf

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestReaderMetadataAndF32TensorRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.gguf")
	var writer Writer
	writer.String("general.architecture", "gemma3")
	writer.Uint32("general.alignment", Alignment)
	writer.Float32("scale", 1.25)
	writer.Bool("enabled", true)
	writer.Strings("tokens", []string{"a", "b"})
	writer.AddTensor(Tensor{Name: "weight", Dimensions: []uint64{2, 2}, Data: []float32{1, 2, 3, 4}})
	if err := writer.WriteFile(path); err != nil {
		t.Fatal(err)
	}

	reader, err := OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if reader.Version() != Version {
		t.Fatalf("version = %d, want %d", reader.Version(), Version)
	}
	if architecture, _ := reader.String("general.architecture"); architecture != "gemma3" {
		t.Fatalf("architecture = %q", architecture)
	}
	if count, _ := reader.ArrayLength("tokens"); count != 2 {
		t.Fatalf("token count = %d, want 2", count)
	}
	data, err := reader.ReadTensorF32("weight")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(data, []float32{1, 2, 3, 4}) {
		t.Fatalf("tensor data = %v", data)
	}
}

func TestReaderRejectsTruncatedAndUnsupportedPayloads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.gguf")
	var writer Writer
	writer.AddTensor(Tensor{Name: "weight", Dimensions: []uint64{1}, Data: []float32{1}})
	data, err := writer.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// This fixture's aligned data section begins at byte 64. Retain only three
	// of the tensor's four F32 bytes.
	if err := os.WriteFile(path, data[:67], 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadTensorF32("weight"); err == nil || !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("truncated payload error = %v", err)
	}
	_ = reader.Close()

	// The one-tensor descriptor places the GGML type immediately before its
	// uint64 offset. Locate the final 12 descriptor bytes before alignment.
	descriptorType := 4 + 4 + 8 + 8 + 8 + len("weight") + 4 + 8
	binary.LittleEndian.PutUint32(data[descriptorType:], 1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err = OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.ReadTensorF32("weight"); err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("unsupported payload error = %v", err)
	}
}

func TestReaderDequantizesQ8Zero(t *testing.T) {
	path := filepath.Join(t.TempDir(), "q8.gguf")
	var writer Writer
	writer.AddTensor(Tensor{Name: "weight", Dimensions: []uint64{32}, Data: make([]float32, 32)})
	data, err := writer.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	// One tensor without metadata has its GGML type at byte 50 and data at 64.
	binary.LittleEndian.PutUint32(data[50:], 8)
	quantized := make([]byte, 34)
	binary.LittleEndian.PutUint16(quantized, 0x3c00) // float16(1).
	for index := range 32 {
		quantized[index+2] = byte(int8(index - 16))
	}
	data = append(data[:64], quantized...)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	reader, err := OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	values, err := reader.ReadTensorF32("weight")
	if err != nil {
		t.Fatal(err)
	}
	if values[0] != -16 || values[31] != 15 {
		t.Fatalf("Q8 values = (%g, %g), want (-16, 15)", values[0], values[31])
	}
}
