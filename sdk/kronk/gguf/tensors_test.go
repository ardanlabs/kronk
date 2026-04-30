package gguf

import "testing"

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
