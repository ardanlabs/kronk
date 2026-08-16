package mtp

import (
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestSynchronize(t *testing.T) {
	tests := []struct {
		name       string
		sharedKV   bool
		wantHidden []float32
	}{
		{"own KV mirrors shifted rows", false, []float32{5, 6, 10, 11, 20, 21}},
		{"shared KV skips mirror", true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var mirrored []float32
			result, err := Synchronize(SyncInput{
				Tokens:        []llama.Token{1, 2, 3},
				HiddenRows:    []float32{10, 11, 20, 21, 30, 31},
				PendingHidden: []float32{5, 6},
				BasePosition:  7,
				EmbeddingSize: 2,
				ChunkSize:     2,
				SharedKV:      tt.sharedKV,
				DecodeOwnChunk: func(_ []llama.Token, _ llama.Pos, hiddenRows []float32, _ bool) error {
					mirrored = append(mirrored, hiddenRows...)
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
			if result.Position != 10 {
				t.Errorf("Position = %d, want 10", result.Position)
			}
		})
	}
}
