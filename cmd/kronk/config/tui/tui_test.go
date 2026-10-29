package tui

import (
	"context"
	"errors"
	"testing"

	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
)

func TestSaveErrorReturnsToReview(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.step, a.path, a.otherPath = stepSaving, "/root/model_config.yaml", "./mine.yaml"

	a.Update(savedMsg{err: errors.New("permission denied")})

	if a.step != stepReview || a.action != actOther || a.otherPath != "./mine.yaml" {
		t.Fatalf("step = %v, action = %q, path = %q; want the review on another file, path kept", a.step, a.action, a.otherPath)
	}
	if a.saveErr == nil {
		t.Fatal("the save error is not shown")
	}
}

func TestBadMediaAnswersAreProblems(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.choice = source.Choice{ID: "whisper", Kind: source.KindAudioModel, Installed: true, Validated: true}
	a.facts = source.Facts{ID: a.choice.ID}
	a.qs = questions.Audio()
	a.ans = questions.NewAnswers(a.qs)
	a.existing = map[string]any{"bogus-key": 1}

	a.questionsDone()

	if a.step != stepReview || len(a.problems) != 1 {
		t.Fatalf("step = %v, problems = %v; want the review with the decode error", a.step, a.problems)
	}
}

func TestCatalogFactsErrorReturnsToPick(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.step, a.choice = stepFacts, source.Choice{ID: "some/model"}

	a.Update(factsMsg{err: errors.New("timeout")})

	if a.step != stepPick || a.formErr == nil || a.formErr.Error() != "timeout" {
		t.Fatalf("step = %v, err = %v; want the picker with the error alone", a.step, a.formErr)
	}
}
