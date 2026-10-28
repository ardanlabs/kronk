package modelprofile_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
)

func TestDecisionContracts(t *testing.T) {
	for _, tt := range []struct {
		protocol string
		want     modelprofile.DecisionExecution
	}{
		{"", modelprofile.DecisionExecution{}},
		{"unknown", modelprofile.DecisionExecution{}},
		{"lev", modelprofile.DecisionExecution{}},
		{"pplx-decider", modelprofile.DecisionExecution{}},
		{"lfm2-d1", modelprofile.DecisionExecution{}},
		{"laya", modelprofile.DecisionExecution{Batch: modelprofile.DecisionBatchPhysical, Embeddings: true}},
		{"kev", modelprofile.DecisionExecution{Batch: modelprofile.DecisionBatchPhysical, Embeddings: true}},
		{"clef", modelprofile.DecisionExecution{Batch: modelprofile.DecisionBatchPhysical, Embeddings: true, AllTokenOutputs: true}},
		{"vllm-decision2", modelprofile.DecisionExecution{Batch: modelprofile.DecisionBatchWholePrompt, Embeddings: true, AllTokenOutputs: true}},
		{"vllm-d3", modelprofile.DecisionExecution{Batch: modelprofile.DecisionBatchWholePrompt, SingleSequence: true, NonCausal: true}},
		{"lfm3-d1", modelprofile.DecisionExecution{Batch: modelprofile.DecisionBatchWholePrompt, SingleSequence: true, NonCausal: true}},
		{"lfm3-d1-omni", modelprofile.DecisionExecution{Batch: modelprofile.DecisionBatchWholePrompt, SingleSequence: true, NonCausal: true}},
	} {
		t.Run(tt.protocol, func(t *testing.T) {
			contract := modelprofile.DecisionContractFor(tt.protocol)
			if contract.Execution != tt.want {
				t.Errorf("Execution: got %+v, want %+v", contract.Execution, tt.want)
			}
			if want := tt.protocol == "vllm-d3"; contract.InstructionsOptional != want {
				t.Errorf("InstructionsOptional: got %t, want %t", contract.InstructionsOptional, want)
			}
			for _, bucket := range []struct {
				count         int
				standard, lev string
			}{
				{-1, "2", "small"}, {0, "2", "small"}, {2, "2", "small"},
				{3, "3_5", "small"}, {5, "3_5", "small"}, {6, "6_10", "small"},
				{8, "6_10", "small"}, {9, "6_10", "mid"}, {10, "6_10", "mid"},
				{11, "11", "mid"}, {26, "11", "mid"}, {27, "11", "large"},
			} {
				want := bucket.standard
				if tt.protocol == "lev" {
					want = bucket.lev
				}
				if got := contract.TemperatureBucket(bucket.count); got != want {
					t.Errorf("TemperatureBucket(%d): got %q, want %q", bucket.count, got, want)
				}
			}
		})
	}
}

func TestDecisionTemperaturePrecision(t *testing.T) {
	for _, tt := range []struct {
		protocol string
		bits     int
	}{
		{"pplx-decider", 32}, {"lfm2-d1", 32}, {"lfm3-d1", 32},
		{"lfm3-d1-omni", 32},
		{"vllm-decision2", 32}, {"vllm-d3", 32}, {"lev", 64},
		{"laya", 64}, {"kev", 64}, {"clef", 64}, {"unknown", 64}, {"", 64},
	} {
		t.Run(tt.protocol, func(t *testing.T) {
			for _, value := range []string{"0.123456789012345", "1e39", "1e-46", "0", "-1", "NaN", "+Inf", "invalid", ""} {
				decision := modelprofile.ResolveDecision(map[string]string{
					"test.decision.type":               tt.protocol,
					"test.decision.temperature.choice": value,
				}, "test")
				want, parseErr := strconv.ParseFloat(value, tt.bits)
				wantErr := parseErr != nil || want <= 0 || value == "NaN" || value == "+Inf"
				got, exists, err := decision.Temperature("choice")
				if !exists || (err != nil) != wantErr || (!wantErr && got != want) {
					t.Errorf("Temperature(%q): got %v/%t/%v, want %v/true/error=%t", value, got, exists, err, want, wantErr)
				}
				if err != nil && !strings.Contains(err.Error(), "test.decision.temperature.choice="+strconv.Quote(value)) {
					t.Errorf("Temperature error: got %v, want original key and value", err)
				}
				if got, exists, err := decision.Temperature("missing"); got != 0 || exists || err != nil {
					t.Errorf("missing Temperature: got %v/%t/%v, want 0/false/nil", got, exists, err)
				}
			}
		})
	}
}
