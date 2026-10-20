package mtp

import (
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestSynchronize(t *testing.T) {
	tests := []struct {
		name         string
		sharedKV     bool
		nextPosition llama.Pos
		wantHidden   []float32
	}{
		{"own KV mirrors shifted rows", false, 10, []float32{5, 6, 10, 11, 20, 21}},
		{"shared KV skips mirror", true, 10, nil},
		{"own KV logical position differs from row count", false, 19, []float32{5, 6, 10, 11, 20, 21}},
		{"shared KV logical position differs from row count", true, 19, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mirrored []float32
			if !tt.sharedKV {
				mirrored = make([]float32, 6)
			}
			result, err := Synchronize(SyncInput{
				RowCount:      3,
				HiddenRows:    []float32{10, 11, 20, 21, 30, 31},
				PendingHidden: []float32{5, 6},
				NextPosition:  tt.nextPosition,
				EmbeddingSize: 2,
				ChunkSize:     2,
				SharedKV:      tt.sharedKV,
				DecodeOwnChunk: func(start, end int, hiddenRows []float32) error {
					if tt.sharedKV {
						t.Fatal("shared KV must not replay rows")
					}
					copy(mirrored[start*2:end*2], hiddenRows)
					return nil
				},
			})
			if err != nil {
				t.Fatalf("Synchronize() error = %v, want nil", err)
			}
			if !slices.Equal(mirrored, tt.wantHidden) {
				t.Errorf("mirrored hidden rows = %v, want %v", mirrored, tt.wantHidden)
			}
			if want := []float32{30, 31}; !slices.Equal(result.PendingHidden, want) {
				t.Errorf("PendingHidden = %v, want %v", result.PendingHidden, want)
			}
			if result.Position != tt.nextPosition {
				t.Errorf("Position = %d, want %d", result.Position, tt.nextPosition)
			}
		})
	}
}
