package gguf

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWriterIsDeterministicAndRefusesOverwrite(t *testing.T) {
	var writer Writer
	writer.String("general.type", "adapter")
	writer.Uint32("general.alignment", Alignment)
	writer.Float32("adapter.lora.alpha", 8)
	writer.AddTensor(Tensor{Name: "blk.0.attn_q.weight.lora_a", Dimensions: []uint64{3, 2}, Data: []float32{1, 2, 3, 4, 5, 6}})

	first, err := writer.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	second, err := writer.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("GGUF encoding is not deterministic")
	}

	path := filepath.Join(t.TempDir(), "adapter.gguf")
	if err := writer.WriteFile(path); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteFile(path); err == nil {
		t.Fatal("WriteFile overwrote an existing adapter")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("adapter permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestWriterRejectsMalformedTensors(t *testing.T) {
	tests := []Tensor{
		{Name: "", Dimensions: []uint64{1}, Data: []float32{1}},
		{Name: "weight", Dimensions: nil, Data: []float32{1}},
		{Name: "weight", Dimensions: []uint64{0}, Data: nil},
		{Name: "weight", Dimensions: []uint64{2}, Data: []float32{1}},
	}
	for _, tensor := range tests {
		var writer Writer
		writer.AddTensor(tensor)
		if _, err := writer.Bytes(); err == nil {
			t.Fatalf("Bytes accepted malformed tensor %+v", tensor)
		}
	}
}
