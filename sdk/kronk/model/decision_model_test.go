//go:build decision_integration

package model_test

import (
	"context"
	"math"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// TestDecisionModels runs with make test-decision, outside ordinary and GitHub
// tests. make install-test-decision installs the pinned libraries and downloads
// fixtures into the normal $HOME/.kronk store; allow tens of gigabytes of disk.
// Only one model is loaded at a time. Missing fixtures fail rather than skip.
//
// This checks text decisions and concurrent state isolation, not media support,
// llama-server parity, or comprehensive decision quality. For a focused run:
// GITHUB_WORKSPACE="$PWD" go test -v -count=1 -tags=decision_integration -run '^TestDecisionModels$/^jev-style$' ./sdk/kronk/model
func TestDecisionModels(t *testing.T) {
	mdls, err := models.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct {
		protocol model.DecisionProtocol
		modelID  string
	}{
		{model.DecisionProtocolOpenJEV, "openjev/OpenJev-Q4_K_M"},
		{model.DecisionProtocolOpenJEV, "ggml-org/OpenJev-Q4_K_M"},
		{model.DecisionProtocolJevStyle, "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0"},
		{model.DecisionProtocolLaya, "ggml-org/Laya-Q8_0"},
		{model.DecisionProtocolLaya, "ggml-org/Julia-1-Q8_0"},
		{model.DecisionProtocolLev, "ggml-org/lev-Q4_K_M"},
		{model.DecisionProtocolKev, "ggml-org/Kev-4B-Q4_K_M"},
		{model.DecisionProtocolNimble, "ggml-org/Bespoke-Nimble-9B-v3-Q4_K_M"},
		{model.DecisionProtocolClef, "ggml-org/Clef-Flash-Q4_K_M"},
		{model.DecisionProtocolClef, "ggml-org/Clef-Q4_K_M"},
		{model.DecisionProtocolPplxDecider, "turintech/pplx-decider-27B-Artemis-IQ3_M"},
		{model.DecisionProtocolGraniteSwitch, "ibm-granite/granite-switch-4.1-8b-Q4_K_M"},
		{model.DecisionProtocolH2OLightning, "h2oai/h2o-lightning-4b-Q4_K_M"},
		{model.DecisionProtocolLfm2D1, "LiquidAI/d1-3B-Q4_K_M"},
		{model.DecisionProtocolLfm2D1Omni, "LiquidAI/d1-omni-600M-Q8_0"},
		{model.DecisionProtocolLfm3D1, "LiquidAI/d1-2B-Q4_K_M"},
		{model.DecisionProtocolLfm3D1Omni, "LiquidAI/d1-Omni-2B-Q8_0"},
		{model.DecisionProtocolVllmD3, "ggml-org/vllm-d3-edge-Q8_0"},
		{model.DecisionProtocolVllmDecision2, "vllm-sr/Decision-2.0-Kai-0.6B-Q8_0"},
	} {
		t.Run(fixture.protocol.String()+"/"+fixture.modelID, func(t *testing.T) {
			if fixture.protocol == model.DecisionProtocolVllmDecision2 {
				t.Skip("published Kai GGUF requires a llama.cpp fork; enable when an upstream-compatible vllm-decision2 GGUF is available")
			}
			mp, err := mdls.FullPath(fixture.modelID)
			if err != nil || len(mp.ModelFiles) == 0 {
				t.Fatalf("missing fixture %s: %v; run make install-test-decision", fixture.modelID, err)
			}
			if err := kronk.Init(); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()
			// Exercise multiple sequences where the protocol supports them;
			// AutoTune chooses capacity for production, not this isolation test.
			const count = 4
			krn, err := kronk.NewWithContext(ctx,
				model.WithModelFiles(mp.ModelFiles), model.WithAutoTune(true), model.WithNSeqMax(count))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				if err := krn.Unload(ctx); err != nil {
					t.Errorf("unload: %v", err)
				}
			})
			// Do not force the protocol: loading the wrong GGUF or breaking its
			// automatic detection must fail this suite too.
			cfg := krn.ModelConfig()
			t.Logf("autotune: applied=%t context=%d prefill=%d physical=%d sequences=%d",
				cfg.AutoTuned, cfg.ContextWindow(), cfg.PrefillBatchSize(), cfg.EffectiveNUBatch(), cfg.NSeqMax())
			if got := cfg.DecisionProtocol; got != fixture.protocol {
				t.Fatalf("detected protocol: got %v, want %v", got, fixture.protocol)
			}

			cases := []struct {
				state any
				shape string
			}{
				{model.DecisionState(model.DecisionStateData("color", "red"), model.DecisionStateData("shape", "triangle")), "triangle"},
				{"The object is a blue circle.", "circle"},
			}
			serial := make([]model.DecisionResponse, len(cases))
			for i, tc := range cases {
				response, err := krn.Decision(ctx, decisionModelRequest(tc.state))
				if err != nil {
					t.Fatalf("serial %s: %v", tc.shape, err)
				}
				checkDecisionModelResponse(t, krn.ModelID(), response, tc.shape)
				t.Logf("serial %s: %+v", tc.shape, response)
				serial[i] = response
			}

			// Mix distinct states and repeated requests behind a common start
			// gate. Compare every typed answer with its isolated baseline.
			concurrent := make([]model.DecisionResponse, count)
			errs := make([]error, count)
			start := make(chan struct{})
			var wg sync.WaitGroup
			for i := range count {
				wg.Go(func() {
					<-start
					concurrent[i], errs[i] = krn.Decision(ctx, decisionModelRequest(cases[i%len(cases)].state))
				})
			}
			close(start)
			wg.Wait()
			for i, response := range concurrent {
				if errs[i] != nil {
					t.Errorf("concurrent request[%d]: %v", i, errs[i])
					continue
				}
				checkDecisionModelResponse(t, krn.ModelID(), response, cases[i%len(cases)].shape)
				checkDecisionModelRepeat(t, serial[i%len(cases)], response)
			}
		})
	}
}

func decisionModelRequest(state any) model.DecisionRequest {
	return model.DecisionRequest{
		State: state,
		Questions: []model.DecisionQuestion{
			model.DecisionQuestionChoice("shape", "What shape is the object?",
				model.DecisionQuestionOpt("square", "Square"),
				model.DecisionQuestionOpt("triangle", "Triangle"),
				model.DecisionQuestionOpt("circle", "Circle")),
			model.DecisionQuestionScore("triangle_score", "Is the shape a triangle?", "No", "Unknown", "Yes"),
			model.DecisionQuestionNoul("triangle", "Is the shape a triangle?", nil),
			model.DecisionQuestionNoul("circle", "Is the shape a circle?", nil),
		},
	}
}

func checkDecisionModelResponse(t *testing.T, modelID string, response model.DecisionResponse, shape string) {
	t.Helper()
	maxFalseProbability := .5
	// d1-omni-600M gives the circle a borderline triangle answer in both Kronk
	// and llama-server b12223 (Boolean 0.5132, score Yes 0.5272). Allow an
	// inconclusive negative below 60%; do not change positive-answer checks.
	if modelID == "d1-omni-600M-Q8_0" {
		maxFalseProbability = .6
	}
	if response.Model != modelID || response.Usage.InputTokens <= 0 || response.Usage.OutputTokens != 0 || len(response.Answers) != 4 {
		t.Errorf("invalid decision envelope: %+v", response)
	}
	for _, question := range decisionModelRequest(nil).Questions {
		answer, exists := response.Answers[question.ID]
		if !exists || answer.Type != question.Type {
			t.Errorf("answer %q: got type %v (exists=%t), want %v", question.ID, answer.Type, exists, question.Type)
			continue
		}
		switch question.Type {
		case model.DecisionQuestionTypeNoul:
			value := answer.Noul
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
				t.Errorf("%s: got Noul %g, want finite [0,1]", question.ID, value)
			}
			// Julia's current checkpoint misclassified the original color Boolean
			// fixtures in both Python and pinned llama.cpp. Retain validity and
			// repeat checks, but omit Boolean accuracy assertions for this model.
			if modelID == "Julia-1-Q8_0" {
				continue
			}
			if question.ID == shape && value <= .5 {
				t.Errorf("%s object: got %s probability %g, want above 0.5", shape, question.ID, value)
			}
			if question.ID != shape && value >= maxFalseProbability {
				t.Errorf("%s object: got %s probability %g, want below %g", shape, question.ID, value, maxFalseProbability)
			}
		default:
			labels := []string{"square", "triangle", "circle"}
			if question.Type == model.DecisionQuestionTypeScore {
				labels = []string{"0", "1", "2"}
			}
			if len(answer.Probabilities) != len(labels) {
				t.Errorf("%s: got %d probabilities, want %d", question.ID, len(answer.Probabilities), len(labels))
			}
			var sum float64
			for _, label := range labels {
				value, exists := answer.Probabilities[label]
				if !exists || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
					t.Errorf("%s probability %q: got %g (exists=%t), want finite [0,1]", question.ID, label, value, exists)
				}
				sum += value
			}
			// Probabilities are rounded to four decimals by the public API.
			if math.Abs(sum-1) > .00015+1e-9 {
				t.Errorf("%s: got probability sum %g, want 1 within rounding tolerance", question.ID, sum)
			}
			if value := answer.Confidence; math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
				t.Errorf("%s: got confidence %g, want finite [0,1]", question.ID, value)
			}
		}
	}
	if got := response.Answers["shape"].Choice; got != shape {
		t.Errorf("choice: got %q, want %q", got, shape)
	}
	score := response.Answers["triangle_score"]
	if value := score.Score; math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 2 {
		t.Errorf("%s object: got triangle score %g, want finite [0,2]", shape, value)
	}
	wrongShape := (shape == "triangle" && score.Score <= 1) || (shape == "circle" && score.Score >= 1)
	if modelID == "d1-omni-600M-Q8_0" && shape == "circle" {
		wrongShape = score.Probabilities["2"] >= maxFalseProbability
	}
	if wrongShape {
		t.Errorf("%s object: triangle score inconsistent with state: %+v", shape, score)
	}
	levels := []string{"No", "Unknown", "Yes"}
	if len(score.Legend) != len(levels) {
		t.Errorf("score legend: got %v, want three levels", score.Legend)
	}
	for i, level := range levels {
		if got := score.Legend[strconv.Itoa(i)]; got != level {
			t.Errorf("score legend[%d]: got %v, want %q", i, got, level)
		}
	}
}

func checkDecisionModelRepeat(t *testing.T, want, got model.DecisionResponse) {
	t.Helper()
	const tolerance = .01
	for id, expected := range want.Answers {
		actual := got.Answers[id]
		if actual.Choice != expected.Choice {
			t.Errorf("repeat %s choice: got %q, want %q", id, actual.Choice, expected.Choice)
		}
		delta := max(math.Abs(actual.Score-expected.Score), math.Abs(actual.Noul-expected.Noul), math.Abs(actual.Confidence-expected.Confidence))
		for label, probability := range expected.Probabilities {
			delta = max(delta, math.Abs(actual.Probabilities[label]-probability))
		}
		if delta > tolerance {
			t.Errorf("repeat %s: maximum delta %g exceeds %g; got %+v, want %+v", id, delta, tolerance, actual, expected)
		}
	}
}
