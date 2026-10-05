package model

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

func TestPlanBatchContribution(t *testing.T) {
	tests := []struct {
		name          string
		inputMixing   modelprofile.BatchInputMixing
		requirements  batchContributionRequirements
		availableRows int
		prefillLimit  int
		want          batchContributionPlan
	}{
		{
			name:          "linear text shares remaining tray",
			requirements:  batchContributionRequirements{input: batchInputTokens, positions: batchPositionLinear, attention: batchAttentionCausal, rows: 3000},
			availableRows: 2044,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionShared, rows: 2044},
		},
		{
			name:          "M-RoPE text uses shared tray",
			requirements:  batchContributionRequirements{input: batchInputTokens, positions: batchPositionMRoPE, attention: batchAttentionCausal, rows: 32},
			availableRows: 2048,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionShared, rows: 32},
		},
		{
			name:          "supported causal embeddings share tray",
			inputMixing:   modelprofile.BatchInputMixingSupported,
			requirements:  batchContributionRequirements{input: batchInputEmbeddings, positions: batchPositionMRoPE, attention: batchAttentionCausal, rows: 1024, mustFit: true},
			availableRows: 2048,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionShared, rows: 1024},
		},
		{
			name:          "unsupported embeddings stay isolated",
			inputMixing:   modelprofile.BatchInputMixingUnsupported,
			requirements:  batchContributionRequirements{input: batchInputEmbeddings, positions: batchPositionLinear, attention: batchAttentionCausal, rows: 1024, mustFit: true},
			availableRows: 2048,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionIsolated, rows: 1024},
		},
		{
			name:          "unknown embeddings stay isolated",
			inputMixing:   modelprofile.BatchInputMixingUnknown,
			requirements:  batchContributionRequirements{input: batchInputEmbeddings, positions: batchPositionLinear, attention: batchAttentionCausal, rows: 1024, mustFit: true},
			availableRows: 2048,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionIsolated, rows: 1024},
		},
		{
			name:          "non-causal embeddings stay isolated",
			inputMixing:   modelprofile.BatchInputMixingSupported,
			requirements:  batchContributionRequirements{input: batchInputEmbeddings, positions: batchPositionLinear, attention: batchAttentionNonCausal, rows: 1024, mustFit: true},
			availableRows: 2048,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionIsolated, rows: 1024},
		},
		{
			name:          "unknown positions stay isolated",
			inputMixing:   modelprofile.BatchInputMixingSupported,
			requirements:  batchContributionRequirements{input: batchInputEmbeddings, positions: batchPositionUnknown, attention: batchAttentionCausal, rows: 1024, mustFit: true},
			availableRows: 2048,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionIsolated, rows: 1024},
		},
		{
			name:          "unknown attention stays isolated",
			inputMixing:   modelprofile.BatchInputMixingSupported,
			requirements:  batchContributionRequirements{input: batchInputEmbeddings, positions: batchPositionLinear, attention: batchAttentionUnknown, rows: 1024, mustFit: true},
			availableRows: 2048,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionIsolated, rows: 1024},
		},
		{
			name:          "whole embeddings stay isolated when tray is short",
			inputMixing:   modelprofile.BatchInputMixingSupported,
			requirements:  batchContributionRequirements{input: batchInputEmbeddings, positions: batchPositionLinear, attention: batchAttentionCausal, rows: 1024, mustFit: true},
			availableRows: 1000,
			prefillLimit:  2048,
			want:          batchContributionPlan{mode: batchExecutionIsolated, rows: 1024},
		},
		{
			name:          "full tray defers text",
			requirements:  batchContributionRequirements{input: batchInputTokens, positions: batchPositionLinear, attention: batchAttentionCausal, rows: 32},
			availableRows: 0,
			prefillLimit:  2048,
			want:          batchContributionPlan{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := planBatchContribution(tt.inputMixing, tt.requirements, tt.availableRows, tt.prefillLimit)
			if got != tt.want {
				t.Errorf("plan = %+v, want %+v", got, tt.want)
			}
		})
	}
}
