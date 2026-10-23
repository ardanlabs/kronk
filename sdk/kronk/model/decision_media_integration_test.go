package model_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// TestClefVision accepts an explicitly provisioned, compatible Clef model and
// projector. Older text-only Clef GGUFs do not have the required image template.
func TestClefVision(t *testing.T) {
	modelFile := os.Getenv("KRONK_TEST_CLEF_MODEL")
	projector := os.Getenv("KRONK_TEST_CLEF_PROJECTOR")
	if modelFile == "" || projector == "" {
		t.Skip("set KRONK_TEST_CLEF_MODEL and KRONK_TEST_CLEF_PROJECTOR to compatible vision fixtures")
	}
	image, err := os.ReadFile(filepath.Join(os.Getenv("GITHUB_WORKSPACE"), "examples/samples/giraffe.jpg"))
	if err != nil {
		t.Fatal(err)
	}
	if err := kronk.Init(); err != nil {
		t.Fatal(err)
	}
	krn, err := kronk.New(model.WithModelFiles([]string{modelFile}), model.WithProjFile(projector),
		model.WithContextWindow(8192), model.WithPrefillBatchSize(8192), model.WithNSeqMax(1),
		model.WithDecisionProtocol(model.DecisionProtocolClef))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("unload: %v", err)
		}
	})
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	req := model.DecisionRequest{
		State: "Identify the animal in the supplied photograph(s).",
		Questions: []model.DecisionQuestion{model.DecisionQuestionChoice("animal", "Which animal is pictured?",
			model.DecisionQuestionOpt("giraffe", "A giraffe"), model.DecisionQuestionOpt("cat", "A domestic cat"))},
	}
	for _, count := range []int{1, 2} {
		req.Images = make([][]byte, count)
		for i := range req.Images {
			req.Images[i] = image
		}
		resp, err := krn.Decision(ctx, req)
		if err != nil {
			t.Fatalf("%d images: %v", count, err)
		}
		if resp.Answers["animal"].Choice != "giraffe" || resp.Usage.InputTokens == 0 || resp.Usage.OutputTokens != 0 {
			t.Fatalf("%d images: got %+v, want giraffe with input-only usage", count, resp)
		}
	}
	req.Images = [][]byte{[]byte("\x89PNG\r\n\x1a\ntruncated")}
	if _, err := krn.Decision(ctx, req); !errors.Is(err, model.ErrDecisionRequest) {
		t.Fatalf("malformed image: got %v, want ErrDecisionRequest", err)
	}
	// A failed image request must not poison later text-only decisions.
	req.Images = nil
	req.State = "A giraffe is standing beside a tree."
	resp, err := krn.Decision(ctx, req)
	if err != nil || resp.Answers["animal"].Choice != "giraffe" {
		t.Fatalf("text after image failure: got %+v, %v", resp, err)
	}
}
