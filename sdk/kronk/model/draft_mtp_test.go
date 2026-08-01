package model

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/gguf"
)

func TestMetadataHasMTP(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		want     bool
	}{
		{"no MTP key", map[string]string{"general.architecture": "qwen35moe"}, false},
		{"zero layers", map[string]string{"qwen35moe.nextn_predict_layers": "0"}, false},
		{"positive layers", map[string]string{"qwen35moe.nextn_predict_layers": "1"}, true},
		{"malformed layers", map[string]string{"qwen35moe.nextn_predict_layers": "invalid"}, false},
		{"architecture-specific key", map[string]string{"cohere2moe.nextn_predict_layers": " 2 "}, true},
		{"matching text within key", map[string]string{"vendor.optional_nextn_predict_layers.count": "3"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := metadataHasMTP(tt.metadata)
			if got != tt.want {
				t.Errorf("metadataHasMTP() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestModelFilesLoadMTPUsesFirstShard(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "model-00001-of-00002.gguf")
	second := filepath.Join(dir, "model-00002-of-00002.gguf")

	writeTestGGUF(t, first, map[string]uint32{"qwen35moe.nextn_predict_layers": 1})
	writeTestGGUF(t, second, nil)

	got, err := modelFilesLoadMTP([]string{first, second})
	if err != nil {
		t.Fatalf("modelFilesLoadMTP() error = %v", err)
	}
	if !got {
		t.Errorf("modelFilesLoadMTP() = false, want true")
	}

	writeTestGGUF(t, first, nil)
	writeTestGGUF(t, second, map[string]uint32{"qwen35moe.nextn_predict_layers": 1})

	got, err = modelFilesLoadMTP([]string{first, second})
	if err != nil {
		t.Fatalf("modelFilesLoadMTP() error = %v", err)
	}
	if got {
		t.Errorf("modelFilesLoadMTP() = true, want false")
	}
}

func writeTestGGUF(t *testing.T, file string, metadata map[string]uint32) {
	t.Helper()

	var data bytes.Buffer
	values := []any{gguf.Magic, uint32(3), uint64(0), uint64(len(metadata))}
	for _, value := range values {
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatalf("binary.Write() error = %v", err)
		}
	}

	for key, value := range metadata {
		if err := binary.Write(&data, binary.LittleEndian, uint64(len(key))); err != nil {
			t.Fatalf("binary.Write() key length error = %v", err)
		}
		if _, err := data.WriteString(key); err != nil {
			t.Fatalf("WriteString() error = %v", err)
		}
		if err := binary.Write(&data, binary.LittleEndian, gguf.MetadataValueTypeUInt32); err != nil {
			t.Fatalf("binary.Write() value type error = %v", err)
		}
		if err := binary.Write(&data, binary.LittleEndian, value); err != nil {
			t.Fatalf("binary.Write() value error = %v", err)
		}
	}

	if err := os.WriteFile(file, data.Bytes(), 0600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
}
