package model

import (
	"testing"

	"github.com/hybridgroup/yzma/pkg/llama"
)

func TestNeedsTargetSpecSnapshot(t *testing.T) {
	tests := []struct {
		name          string
		modelType     ModelType
		rollbackDepth uint32
		draftCount    int
		want          bool
	}{
		{"dense target", ModelTypeDense, 0, 2, false},
		{"hybrid native rollback covers draft", ModelTypeHybrid, 2, 2, false},
		{"hybrid native rollback is insufficient", ModelTypeHybrid, 1, 2, true},
		{"hybrid rollback unavailable", ModelTypeHybrid, 0, 2, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := needsTargetSpecSnapshot(tt.modelType, tt.rollbackDepth, tt.draftCount)
			if got != tt.want {
				t.Errorf("needsTargetSpecSnapshot() = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestMaxDraftForSlot(t *testing.T) {
	e := batchEngine{model: &Model{cfg: Config{PtrContextWindow: new(10)}}}

	tests := []struct {
		name       string
		nPast      llama.Pos
		configured int
		want       int
	}{
		{"full draft fits", 5, 3, 3},
		{"draft capped at window", 8, 3, 1},
		{"only target token fits", 9, 3, 0},
		{"window exhausted", 10, 3, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := slot{nPast: tt.nPast}
			got := e.maxDraftForSlot(&s, tt.configured)
			if got != tt.want {
				t.Errorf("maxDraftForSlot() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestClassicDraftKeepPosition(t *testing.T) {
	tests := []struct {
		name      string
		startPast llama.Pos
		endPast   llama.Pos
		accepted  int
		want      llama.Pos
	}{
		{"partial acceptance", 100, 104, 1, 102},
		{"full acceptance", 100, 104, 4, 104},
		{"EOG-truncated full acceptance", 100, 103, 2, 103},
		{"EOG-truncated partial acceptance", 100, 103, 1, 102},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classicDraftKeepPosition(tt.startPast, tt.endPast, tt.accepted)
			if got != tt.want {
				t.Errorf("classicDraftKeepPosition() = %d, want %d", got, tt.want)
			}
		})
	}
}
