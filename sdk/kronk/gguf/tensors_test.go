package gguf

import "testing"

func TestGGMLTypeSizes(t *testing.T) {
	tests := []struct {
		name      string
		ggmlType  uint32
		blockSize int64
		typeSize  int64
	}{
		{"F32", 0, 1, 4},
		{"F16", 1, 1, 2},
		{"Q4_0", 2, 32, 18},
		{"Q4_1", 3, 32, 20},
		{"Q5_0", 6, 32, 22},
		{"Q5_1", 7, 32, 24},
		{"Q8_0", 8, 32, 34},
		{"Q8_1", 9, 32, 36},
		{"Q2_K", 10, 256, 84},
		{"Q3_K", 11, 256, 110},
		{"Q4_K", 12, 256, 144},
		{"Q5_K", 13, 256, 176},
		{"Q6_K", 14, 256, 210},
		{"Q8_K", 15, 256, 292},
		{"IQ2_XXS", 16, 256, 66},
		{"IQ2_XS", 17, 256, 74},
		{"IQ3_XXS", 18, 256, 98},
		{"IQ1_S", 19, 256, 50},
		{"IQ4_NL", 20, 32, 18},
		{"IQ3_S", 21, 256, 110},
		{"IQ2_S", 22, 256, 82},
		{"IQ4_XS", 23, 256, 136},
		{"I8", 24, 1, 1},
		{"I16", 25, 1, 2},
		{"I32", 26, 1, 4},
		{"I64", 27, 1, 8},
		{"F64", 28, 1, 8},
		{"IQ1_M", 29, 256, 56},
		{"BF16", 30, 1, 2},
		{"TQ1_0", 34, 256, 54},
		{"TQ2_0", 35, 256, 66},
		{"MXFP4", 39, 32, 17},
		{"NVFP4", 40, 64, 36},
		{"Q1_0", 41, 128, 18},
		{"Q2_0", 42, 64, 18},
	}

	if got, want := len(ggmlTypeSizes), len(tests); got != want {
		t.Fatalf("ggmlTypeSizes length: got %d, want %d", got, want)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, exists := ggmlTypeSizes[tt.ggmlType]
			if !exists {
				t.Fatalf("ggmlTypeSizes[%d]: entry is missing", tt.ggmlType)
			}
			if got.blockSize != tt.blockSize {
				t.Errorf("blockSize: got %d, want %d", got.blockSize, tt.blockSize)
			}
			if got.typeSize != tt.typeSize {
				t.Errorf("typeSize: got %d, want %d", got.typeSize, tt.typeSize)
			}
		})
	}
}

func TestGGMLRowSize(t *testing.T) {
	tests := []struct {
		name     string
		ggmlType uint32
		ne0      int64
		want     int64
	}{
		{"F32-128", 0, 128, 512},
		{"F16-128", 1, 128, 256},
		{"Q4_0-128", 2, 128, 72},
		{"Q8_0-128", 8, 128, 136},
		{"BF16-128", 30, 128, 256},
		{"Q4_0-4096", 2, 4096, 2304},
		{"MXFP4-128", 39, 128, 68},
		{"NVFP4-128", 40, 128, 72},
		{"Q1_0-128", 41, 128, 18},
		{"Q2_0-128", 42, 128, 36},
		{"unknown-type", 255, 128, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GGMLRowSize(tt.ggmlType, tt.ne0)
			if got != tt.want {
				t.Errorf("GGMLRowSize(%d, %d) = %d, want %d", tt.ggmlType, tt.ne0, got, tt.want)
			}
		})
	}
}

func TestGGMLTensorSize(t *testing.T) {
	tests := []struct {
		name     string
		ggmlType uint32
		dims     []int64
		want     int64
	}{
		{"F16-2D-4096x4096", 1, []int64{4096, 4096}, 4096 * 2 * 4096},
		{"Q4_0-2D-4096x4096", 2, []int64{4096, 4096}, 2304 * 4096},
		{"F32-1D-128", 0, []int64{128}, 512},
		{"MXFP4-2D-4096x4096", 39, []int64{4096, 4096}, 2176 * 4096},
		{"empty-dims", 0, []int64{}, 0},
		{"F16-3D", 1, []int64{128, 32, 8}, 128 * 2 * 32 * 8},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GGMLTensorSize(tt.ggmlType, tt.dims)
			if got != tt.want {
				t.Errorf("GGMLTensorSize(%d, %v) = %d, want %d", tt.ggmlType, tt.dims, got, tt.want)
			}
		})
	}
}
