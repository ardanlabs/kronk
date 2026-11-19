package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/ardanlabs/kronk/cmd/kronk/config/questions"
	"github.com/ardanlabs/kronk/cmd/kronk/config/source"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

func TestErrComesFromFail(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	if err := Err(a); err != nil {
		t.Fatalf("err = %v, want none", err)
	}

	want := errors.New("no catalog")
	a.fail(want)

	if err := Err(a); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

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
	a.choice = source.Choice{ID: "whisper", Kind: source.KindAudioModel, Installed: true}
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

func TestListSkipsToTheReview(t *testing.T) {
	a := newApp(context.Background(), nil, true)
	a.choice = source.Choice{ID: "whisper", Kind: source.KindAudioModel, Installed: true}
	a.facts = source.Facts{ID: a.choice.ID}
	a.qs = questions.Audio()
	a.ans = questions.NewAnswers(a.qs)
	a.openQuestions()

	a.openList()
	rows := a.listRows()
	if rows[len(rows)-1].key != rowReview {
		t.Fatalf("last row = %q, want the review row", rows[len(rows)-1].key)
	}

	a.listKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	a.listKey(tea.KeyPressMsg{Code: tea.KeyEnter})

	if a.step != stepReview || a.list.open {
		t.Fatalf("step = %v, list open = %v; want the review", a.step, a.list.open)
	}
}

func TestQuitWithPendingChanges(t *testing.T) {
	for _, action := range []string{"save", "discard", "continue", "invalid", "unchanged", "save-error"} {
		t.Run(action, func(t *testing.T) {
			a := newApp(t.Context(), nil, true)
			a.path = filepath.Join(t.TempDir(), "model_config.yaml")
			const original = "version: 2\nbucky-models:\n  whisper:\n    nthreads: 3\n"
			if err := os.WriteFile(a.path, []byte(original), 0o600); err != nil {
				t.Fatal(err)
			}
			doc, err := modelconfig.Load(a.path)
			if err != nil {
				t.Fatal(err)
			}
			a.doc = doc
			a.choice = source.Choice{ID: "whisper", Kind: source.KindAudioModel, Installed: true}
			a.entryID, a.section = "whisper", "bucky-models"
			a.startMediaQuestions()
			if got := a.ans.Value("nthreads"); got != "3" {
				t.Fatalf("existing nthreads = %q, want 3", got)
			}
			if action != "unchanged" {
				*a.ans.Sel["nthreads"] = "7"
			}
			if action == "invalid" {
				*a.ans.Sel["nthreads"] = "not a number"
			}
			_, cmd := a.Update(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
			if action == "unchanged" {
				if cmd == nil {
					t.Fatal("unchanged entry did not quit")
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("unchanged entry prompted instead of quitting")
				}
				return
			}
			if a.step != stepQuit {
				t.Fatalf("step = %v, want quit confirmation", a.step)
			}
			if !strings.Contains(a.View().Content, "Save your current changes") {
				t.Fatal("quit confirmation was not rendered")
			}
			switch action {
			case "discard":
				a.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
				_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if cmd == nil {
					t.Fatal("discard did not quit")
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("discard did not return quit")
				}
			case "continue":
				a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if a.step != stepQuestions || a.ans.Value("nthreads") != "7" {
					t.Fatal("continuing lost pending answers")
				}
			default:
				a.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
				a.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
				_, cmd = a.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
				if action == "invalid" {
					if a.step != stepQuestions || focusedKey(a.currentForm()) != "nthreads" {
						t.Fatal("invalid answer was not reopened before saving")
					}
					break
				}
				if a.step != stepSaving || cmd == nil {
					t.Fatalf("step = %v, want saving", a.step)
				}
				if action == "save-error" {
					a.Update(savedMsg{err: errors.New("read-only file")})
					if a.step != stepReview || a.saveErr == nil {
						t.Fatal("failed save did not retain the review")
					}
					break
				}
				// Execute the actual save command, then deliver its result.
				_, cmd = a.Update(a.save(a.path)())
				if cmd == nil {
					t.Fatal("successful save did not quit")
				}
				if _, ok := cmd().(tea.QuitMsg); !ok {
					t.Fatal("successful save did not return quit")
				}
			}
			data, err := os.ReadFile(a.path)
			if err != nil {
				t.Fatal(err)
			}
			if action != "save" && string(data) != original {
				t.Fatal("file changed without a successful save")
			}
			if action == "save" {
				saved, err := modelconfig.Load(a.path)
				if err != nil {
					t.Fatal(err)
				}
				if got := *saved.BuckyModels["whisper"].NThreads; got != 7 {
					t.Fatalf("saved nthreads = %d, want 7", got)
				}
			}
		})
	}
}

func TestClearStoredSecret(t *testing.T) {
	a := newApp(t.Context(), nil, true)
	a.path = filepath.Join(t.TempDir(), "model_config.yaml")
	if err := os.WriteFile(a.path, []byte("version: 2\nkms:\n  hf-token: private-token\nmodels: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.start = newStart(a.path)
	a.start.what = whatServer
	a.startDone()
	a.openAt(stepQuestions, "hf-token")
	if strings.Contains(a.View().Content, "private-token") {
		t.Fatal("stored secret was exposed")
	}
	a.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	if !strings.Contains(a.View().Content, "will be removed") {
		t.Fatal("secret removal was not explained")
	}
	a.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	if a.ans.Value("hf-token") != "" {
		t.Fatal("toggling removal did not retain the stored secret")
	}
	a.Update(tea.KeyPressMsg{Code: 'x', Mod: tea.ModCtrl})
	a.questionsDone()
	if len(a.changes) != 1 || a.changes[0].Key != "hf-token" || !a.changes[0].Del {
		t.Fatalf("secret removal changes = %+v", a.changes)
	}
	msg := a.save(a.path)().(savedMsg)
	if msg.err != nil {
		t.Fatal(msg.err)
	}
	data, err := os.ReadFile(a.path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "hf-token") {
		t.Fatal("secret remained in the saved file")
	}
}
