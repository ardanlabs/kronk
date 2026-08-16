package mtp

import (
	"slices"
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestGenerate(t *testing.T) {
	tests := []struct {
		name          string
		fixedPosition bool
		wantPositions []llama.Pos
		wantPosition  llama.Pos
	}{
		{"own KV advances", false, []llama.Pos{7, 8, 9}, 10},
		{"shared KV stays fixed", true, []llama.Pos{7, 7, 7}, 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var positions []llama.Pos
			result, err := Generate(DraftInput{
				Token:         10,
				Position:      7,
				Hidden:        []float32{1},
				Count:         3,
				FixedPosition: tt.fixedPosition,
				IsEOG:         func(llama.Token) bool { return false },
				DecodeStep: func(token llama.Token, position llama.Pos, hidden []float32) (llama.Token, []float32, bool, error) {
					positions = append(positions, position)
					return token + 1, []float32{hidden[0] + 1}, true, nil
				},
			})
			if err != nil {
				t.Fatalf("Generate() error = %v, want nil", err)
			}
			if !slices.Equal(positions, tt.wantPositions) {
				t.Errorf("positions = %v, want %v", positions, tt.wantPositions)
			}
			if result.Position != tt.wantPosition {
				t.Errorf("Position = %d, want %d", result.Position, tt.wantPosition)
			}
			if want := []llama.Token{11, 12, 13}; !slices.Equal(result.Candidates, want) {
				t.Errorf("Candidates = %v, want %v", result.Candidates, want)
			}
		})
	}
}

func TestGenerateStopsAtEOG(t *testing.T) {
	hidden := []float32{1}
	result, err := Generate(DraftInput{
		Token:    10,
		Position: 2,
		Hidden:   hidden,
		Count:    3,
		IsEOG:    func(token llama.Token) bool { return token == 11 },
		DecodeStep: func(token llama.Token, _ llama.Pos, _ []float32) (llama.Token, []float32, bool, error) {
			return token + 1, []float32{2}, true, nil
		},
	})
	if err != nil {
		t.Fatalf("Generate() error = %v, want nil", err)
	}
	if want := []llama.Token{11}; !slices.Equal(result.Candidates, want) {
		t.Errorf("Candidates = %v, want %v", result.Candidates, want)
	}
	result.Hidden[0] = 9
	if hidden[0] != 1 {
		t.Errorf("input hidden state = %v, want [1]", hidden)
	}
}
