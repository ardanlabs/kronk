package gguf

import (
	"os"
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/libs"
	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestGGMLTypeSizesMatchLlama(t *testing.T) {
	libPath := libs.Path("")
	if _, err := os.Stat(libPath); err != nil {
		t.Skipf("llama.cpp libraries not installed at %s", libPath)
	}

	if err := llama.Load(libPath); err != nil {
		t.Fatalf("load llama.cpp libraries: %v", err)
	}
	t.Cleanup(llama.Close)

	if got, want := llama.GGMLTypeCOUNT, llama.GGMLType(43); got != want {
		t.Fatalf("GGMLTypeCOUNT: got %d, want %d; review the pure-Go sizing table for new types", got, want)
	}

	for ggmlType, info := range ggmlTypeSizes {
		t.Run(llama.GGMLTypeName(llama.GGMLType(ggmlType)), func(t *testing.T) {
			typ := llama.GGMLType(ggmlType)
			if got := llama.GGMLBlockSize(typ); got != info.blockSize {
				t.Errorf("block size: got %d, want %d", got, info.blockSize)
			}
			if got := llama.GGMLTypeSize(typ); got != uint64(info.typeSize) {
				t.Errorf("type size: got %d, want %d", got, info.typeSize)
			}

			rowElements := info.blockSize * 4
			if got := llama.GGMLRowSize(typ, rowElements); got != uint64(GGMLRowSize(ggmlType, rowElements)) {
				t.Errorf("row size: got %d, want %d", got, GGMLRowSize(ggmlType, rowElements))
			}
		})
	}
}
