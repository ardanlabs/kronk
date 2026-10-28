package model_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

// TestLFM3D1Omni exercises real media through the public SDK. The spoken
// fixture asks "Which animal is in the picture?" in an audio file.
func TestLFM3D1Omni(t *testing.T) {
	if testing.Short() {
		t.Skip("model inference skipped in short mode")
	}
	modelFile := os.Getenv("KRONK_TEST_LFM3_OMNI_MODEL")
	projector := os.Getenv("KRONK_TEST_LFM3_OMNI_PROJECTOR")
	if modelFile == "" && projector == "" {
		mdls, err := models.New()
		if err != nil {
			t.Fatal(err)
		}
		mp, err := mdls.FullPath("LiquidAI/d1-Omni-2B-Q8_0")
		if err != nil {
			if os.Getenv("CI") == "true" {
				t.Fatalf("required LFM3 Omni fixture: %v", err)
			}
			t.Skipf("LFM3 Omni model not installed: %v", err)
		}
		if len(mp.ModelFiles) != 0 {
			modelFile = mp.ModelFiles[0]
		}
		projector = mp.ProjFile
	}
	if modelFile == "" || projector == "" {
		t.Fatal("LFM3 Omni requires both a model and projector")
	}
	if err := kronk.Init(kronk.WithLibPath(os.Getenv("KRONK_TEST_LIB_PATH"))); err != nil {
		t.Fatal(err)
	}
	krn, err := kronk.New(model.WithModelFiles([]string{modelFile}), model.WithProjFile(projector),
		model.WithContextWindow(4096), model.WithPrefillBatchSize(256), model.WithNSeqMax(4),
		model.WithSpeculationMode(model.SpeculationDisabled))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := krn.Unload(context.Background()); err != nil {
			t.Errorf("unload: %v", err)
		}
	})
	if got := krn.ModelConfig().DecisionProtocol; got != model.DecisionProtocolLfm3D1Omni {
		t.Fatalf("detected protocol: got %v, want lfm3-d1-omni", got)
	}
	read := func(t *testing.T, name string) []byte {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(os.Getenv("GITHUB_WORKSPACE"), "examples/samples", name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	animal := model.DecisionQuestionChoice("animal", "Which animal is in the picture?",
		model.DecisionQuestionOpt("giraffe", "A giraffe"), model.DecisionQuestionOpt("deer", "A deer"))
	for _, image := range []struct{ name, want string }{{"giraffe.jpg", "giraffe"}, {"deer/image10.jpg", "deer"}} {
		t.Run(image.want, func(t *testing.T) {
			// Parallel calls exercise independent projector preparations and the
			// decision runtime's single-sequence execution requirement.
			t.Parallel()
			for range 2 {
				resp, err := krn.Decision(t.Context(), model.DecisionRequest{Files: [][]byte{read(t, image.name)}, Questions: []model.DecisionQuestion{animal}})
				if err != nil || resp.Answers["animal"].Choice != image.want || resp.Usage.InputTokens <= 1 || resp.Usage.OutputTokens != 0 {
					t.Fatalf("image decision: got %+v, %v, want %s with input-only usage", resp, err, image.want)
				}
			}
		})
	}
	t.Run("audio state", func(t *testing.T) {
		resp, err := krn.Decision(t.Context(), model.DecisionRequest{Files: [][]byte{read(t, "jfk.wav")}, Questions: []model.DecisionQuestion{
			model.DecisionQuestionChoice("subject", "What is the subject of this speech?",
				model.DecisionQuestionOpt("service", "Serving one's country"), model.DecisionQuestionOpt("cooking", "A recipe for dinner")),
			model.DecisionQuestionNoul("country", "Does the speaker mention their country?", nil),
			model.DecisionQuestionScore("rating", "Does this clip contain human speech?", "No human speech", "Clear human speech"),
		}})
		// Each question has one final readout token; usage must also include
		// the encoded audio and the rest of its rendered text prompt.
		if err != nil || resp.Answers["subject"].Choice != "service" || resp.Answers["country"].Noul <= .5 || resp.Answers["rating"].Score != 1 || resp.Usage.InputTokens <= 3 {
			t.Fatalf("audio decision: got %+v, %v", resp, err)
		}
	})
	t.Run("spoken instructions", func(t *testing.T) {
		file := os.Getenv("KRONK_TEST_LFM3_OMNI_QUESTION")
		if file == "" {
			file = filepath.Join(os.Getenv("GITHUB_WORKSPACE"), "examples/samples/which-animal.wav")
		}
		audio, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		question := animal
		question.Instructions = model.DecisionAudioInstructions{Data: audio}
		for _, image := range []struct{ name, want string }{{"giraffe.jpg", "giraffe"}, {"deer/image10.jpg", "deer"}} {
			resp, err := krn.Decision(t.Context(), model.DecisionRequest{Images: [][]byte{read(t, image.name)}, Questions: []model.DecisionQuestion{question}})
			if err != nil || resp.Answers["animal"].Choice != image.want {
				t.Fatalf("spoken question: got %+v, %v, want %s", resp, err, image.want)
			}
		}
	})
	t.Run("failures and recovery", func(t *testing.T) {
		for _, req := range []model.DecisionRequest{
			{Files: [][]byte{[]byte("RIFF\x00\x00\x00\x00WAVEtruncated")}, Questions: []model.DecisionQuestion{animal}},
			{Questions: []model.DecisionQuestion{model.DecisionQuestionNoul("q", model.DecisionAudioInstructions{}, nil)}},
		} {
			if _, err := krn.Decision(t.Context(), req); !errors.Is(err, model.ErrDecisionRequest) {
				t.Fatalf("malformed media: got %v, want ErrDecisionRequest", err)
			}
		}
		if _, err := krn.Decision(t.Context(), model.DecisionRequest{Files: [][]byte{read(t, "giraffe.jpg")}, State: strings.Repeat("long state ", 5000), Questions: []model.DecisionQuestion{animal}}); !errors.Is(err, model.ErrDecisionBudget) {
			t.Fatalf("media budget: got %v, want ErrDecisionBudget", err)
		}
		resp, err := krn.Decision(t.Context(), model.DecisionRequest{State: "A deer is standing in the forest.", Questions: []model.DecisionQuestion{animal}})
		if err != nil || resp.Answers["animal"].Choice != "deer" {
			t.Fatalf("text recovery: got %+v, %v, want deer", resp, err)
		}
	})
}
