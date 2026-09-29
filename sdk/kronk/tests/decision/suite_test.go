package decision_test

import (
	"context"
	"fmt"
	"math"
	"sync"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

const probabilityTolerance = 0.01

func TestSuite(t *testing.T) {
	if len(testlib.MPDecision.ModelFiles) == 0 {
		t.Skip("model not downloaded")
	}

	testlib.WithModel(t, testlib.CfgDecision(), func(t *testing.T, krn *kronk.Kronk) {
		t.Run("TypedQuestions", func(t *testing.T) {
			testTypedQuestions(t, krn)
		})

		t.Run("ConcurrentRequests", func(t *testing.T) {
			testlib.SkipOnBackends(t, "multi-sequence llama_decode returns corrupted logits", "rocm")
			testConcurrentRequests(t, krn)
		})
	})
}

func testTypedQuestions(t *testing.T, krn *kronk.Kronk) {
	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	response, err := krn.Decision(ctx, model.DecisionRequest{
		State: model.DecisionState(
			model.DecisionStateData("customer_message", "I was charged twice for one subscription renewal."),
			model.DecisionStateData("account_tier", "business"),
		),
		Questions: []model.DecisionQuestion{
			model.DecisionQuestionChoice("route", "Which team should handle this request?",
				model.DecisionQuestionOpt("billing", "Payments, invoices, refunds, and duplicate charges"),
				model.DecisionQuestionOpt("support", "Product bugs and technical problems"),
				model.DecisionQuestionOpt("sales", "Plans, pricing, and new purchases"),
			),
			model.DecisionQuestionScore("urgency", "How urgent is this request?",
				"not urgent", "normal", "urgent", "critical",
			),
			model.DecisionQuestionNoul("human", "Should a human review this request?", nil),
		},
	})
	if err != nil {
		t.Fatalf("Decision: %v", err)
	}

	if response.Model != krn.ModelID() {
		t.Errorf("model: got %q, want %q", response.Model, krn.ModelID())
	}
	if response.Usage.InputTokens == 0 {
		t.Error("input tokens: got 0, want positive")
	}
	route, exists := response.Answers["route"]
	if !exists || route.Type != model.DecisionQuestionTypeChoice {
		t.Fatalf("route: got type %q and exists=%t, want choice answer", route.Type, exists)
	}
	if got := route.Choice; got != "billing" {
		t.Errorf("route: got %q, want billing", got)
	}
	urgency, exists := response.Answers["urgency"]
	if !exists || urgency.Type != model.DecisionQuestionTypeScore {
		t.Fatalf("urgency: got type %q and exists=%t, want score answer", urgency.Type, exists)
	}
	if got := urgency.Score; got < 0 || got > 3 {
		t.Errorf("urgency: got %v, want within [0,3]", got)
	}
	human, exists := response.Answers["human"]
	if !exists || human.Type != model.DecisionQuestionTypeNoul {
		t.Fatalf("human: got type %q and exists=%t, want noul answer", human.Type, exists)
	}
	if got := human.Noul; got < 0 || got > 1 {
		t.Errorf("human: got %v, want within [0,1]", got)
	}
}

func testConcurrentRequests(t *testing.T, krn *kronk.Kronk) {
	requests := []model.DecisionRequest{
		decisionRequest("A customer was charged twice for one subscription renewal."),
		decisionRequest("A customer cannot sign in after resetting their password."),
		decisionRequest("A prospect asks for pricing and wants to upgrade to the enterprise plan."),
		decisionRequest("A customer was charged twice for one subscription renewal."),
	}

	ctx, cancel := context.WithTimeout(context.Background(), testlib.TestDuration)
	defer cancel()

	serial := make([]model.DecisionResponse, len(requests))
	for i, request := range requests {
		response, err := krn.Decision(ctx, request)
		if err != nil {
			t.Fatalf("serial request[%d]: %v", i, err)
		}
		serial[i] = response
	}

	concurrent := make([]model.DecisionResponse, len(requests))
	errs := make([]error, len(requests))
	start := make(chan struct{})

	var wg sync.WaitGroup
	for i, request := range requests {
		wg.Go(func() {
			<-start
			concurrent[i], errs[i] = krn.Decision(ctx, request)
		})
	}
	close(start)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent request[%d]: %v", i, err)
		}
	}
	if t.Failed() {
		return
	}

	for i := range requests {
		if err := compareResponses(serial[i], concurrent[i]); err != nil {
			t.Errorf("request[%d]: %v", i, err)
		}
	}
	if err := compareResponses(concurrent[0], concurrent[len(concurrent)-1]); err != nil {
		t.Errorf("duplicate request: %v", err)
	}
}

func decisionRequest(state string) model.DecisionRequest {
	return model.DecisionRequest{
		State: state,
		Questions: []model.DecisionQuestion{
			model.DecisionQuestionChoice("route", "Which team should handle this request?",
				model.DecisionQuestionOpt("billing", "Payments and duplicate charges"),
				model.DecisionQuestionOpt("accounts", "Login and account access"),
				model.DecisionQuestionOpt("sales", "Plans and pricing"),
			),
		},
	}
}

func compareResponses(want, got model.DecisionResponse) error {
	wantAnswer := want.Answers["route"]
	gotAnswer := got.Answers["route"]
	if gotAnswer.Choice != wantAnswer.Choice {
		return fmt.Errorf("choice: got %q, want %q", gotAnswer.Choice, wantAnswer.Choice)
	}
	if len(gotAnswer.Probabilities) != len(wantAnswer.Probabilities) {
		return fmt.Errorf("probabilities: got %d, want %d", len(gotAnswer.Probabilities), len(wantAnswer.Probabilities))
	}

	var maximumDelta float64
	for name, wantProbability := range wantAnswer.Probabilities {
		gotProbability, exists := gotAnswer.Probabilities[name]
		if !exists {
			return fmt.Errorf("probability %q is missing", name)
		}
		maximumDelta = max(maximumDelta, math.Abs(gotProbability-wantProbability))
	}
	if maximumDelta > probabilityTolerance {
		return fmt.Errorf("maximum probability delta %.8g exceeds %.4g", maximumDelta, probabilityTolerance)
	}

	return nil
}
