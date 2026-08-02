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
		name             string
		nPast            llama.Pos
		configured       int
		maxTokens        int
		reasonTokens     int
		completionTokens int
		want             int
	}{
		{name: "full draft fits", nPast: 5, configured: 3, maxTokens: 100, want: 3},
		{name: "draft capped at two-cell context reserve", nPast: 7, configured: 3, maxTokens: 100, want: 1},
		{name: "context reserve exhausted", nPast: 8, configured: 3, maxTokens: 100, want: 0},
		{name: "window exhausted", nPast: 10, configured: 3, maxTokens: 100, want: 0},
		{name: "draft capped at remaining budget", nPast: 1, configured: 3, maxTokens: 5, completionTokens: 3, want: 1},
		{name: "reasoning and completion share budget", nPast: 1, configured: 3, maxTokens: 5, reasonTokens: 2, completionTokens: 2, want: 0},
		{name: "last output token reserved for target", nPast: 1, configured: 3, maxTokens: 5, completionTokens: 4, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := slot{
				nPast:            tt.nPast,
				reasonTokens:     tt.reasonTokens,
				completionTokens: tt.completionTokens,
				job:              &chatJob{params: Params{MaxTokens: tt.maxTokens}},
			}
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
