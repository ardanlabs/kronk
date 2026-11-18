package providers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// All inference goes through these callbacks; no test loads a native model.
type inferenceStub struct {
	cfg      model.Config
	chat     func(context.Context, model.D) (model.ChatResponse, error)
	tokenize func(context.Context, model.D) (model.TokenizeResponse, error)
	ids      func(context.Context, string) ([]int32, error)
}

func (s inferenceStub) Chat(ctx context.Context, input model.D) (model.ChatResponse, error) {
	return s.chat(ctx, input)
}

func (s inferenceStub) Tokenize(ctx context.Context, input model.D) (model.TokenizeResponse, error) {
	return s.tokenize(ctx, input)
}

func (s inferenceStub) TokenizeIDs(ctx context.Context, input string) ([]int32, error) {
	return s.ids(ctx, input)
}

func (s inferenceStub) ModelConfig() model.Config { return s.cfg }

func TestGoMLXPredictProtectedInput(t *testing.T) {
	for _, adapter := range []string{"", "candidate.gguf"} {
		t.Run("adapter="+adapter, func(t *testing.T) {
			ctx := t.Context()
			job := TrainingJob{ModelID: "owner/base", Model: "base.gguf", Training: Training{Seed: 53}, PredictionTokens: 192}
			records := []Record{
				{Prompt: "First question?", Target: "SECRET TARGET ONE"},
				{Prompt: "Second question?", Target: "SECRET TARGET TWO"},
			}
			original := append([]Record(nil), records...)
			chats, tokens, acquired, released := 0, 0, 0, 0
			stub := inferenceStub{
				cfg: model.Config{PtrContextWindow: new(512)},
				tokenize: func(gotCtx context.Context, input model.D) (model.TokenizeResponse, error) {
					want := model.D{"input": records[tokens].Prompt, "apply_template": true}
					if gotCtx != ctx || !reflect.DeepEqual(input, want) {
						t.Errorf("tokenize: got %v, %#v, want original context, %#v", gotCtx, input, want)
					}
					tokens++
					return model.TokenizeResponse{Tokens: 10}, nil
				},
				chat: func(gotCtx context.Context, input model.D) (model.ChatResponse, error) {
					want := model.D{
						"messages":    []model.D{{"role": "user", "content": records[chats].Prompt}},
						"temperature": 0, "seed": uint32(53), "max_completion_tokens": 192,
						"enable_thinking": false, "stream": false, "repeat_penalty": 1.0, "dry_multiplier": 0.0,
					}
					if gotCtx != ctx || !reflect.DeepEqual(input, want) {
						t.Errorf("chat: got %v, %#v, want original context, %#v", gotCtx, input, want)
					}
					chats++
					return model.ChatResponse{Choices: []model.Choice{{Message: &model.ResponseMessage{Content: " answer "}, FinishReasonPtr: new(model.FinishReasonStop)}}}, nil
				},
			}
			trainer := GoMLX{acquire: func(gotCtx context.Context, gotJob TrainingJob, gotAdapter string) (inferenceModel, func() error, error) {
				acquired++
				if gotCtx != ctx || !reflect.DeepEqual(gotJob, job) || gotAdapter != adapter {
					t.Errorf("acquire: got %v, %+v, %q, want original context, %+v, %q", gotCtx, gotJob, gotAdapter, job, adapter)
				}
				return stub, func() error { released++; return nil }, nil
			}}
			answers, err := trainer.Predict(ctx, job, adapter, records, nil)
			if err != nil || !reflect.DeepEqual(answers, []string{" answer ", " answer "}) {
				t.Fatalf("Predict: got %q, %v, want two unmodified answers, nil", answers, err)
			}
			if acquired != 1 || released != 1 || tokens != 2 || chats != 2 {
				t.Errorf("calls: got acquire/release/tokenize/chat %d/%d/%d/%d, want 1/1/2/2", acquired, released, tokens, chats)
			}
			if !reflect.DeepEqual(records, original) {
				t.Errorf("records: got %+v, want %+v", records, original)
			}
		})
	}
}

// A cut-off or empty answer is kept as given, so evaluation scores it as wrong
// instead of failing the training run.
func TestGoMLXPredictKeepsCutOffAnswers(t *testing.T) {
	responses := []model.Choice{
		{Message: &model.ResponseMessage{Content: "The answer runs on and"}, FinishReasonPtr: new(model.FinishReasonLength)},
		{Message: &model.ResponseMessage{Content: " \n\t"}, FinishReasonPtr: new(model.FinishReasonStop)},
		{Message: &model.ResponseMessage{Content: "no finish reason"}},
	}
	chats := 0
	stub := inferenceStub{
		cfg: model.Config{PtrContextWindow: new(512)},
		tokenize: func(context.Context, model.D) (model.TokenizeResponse, error) {
			return model.TokenizeResponse{Tokens: 1}, nil
		},
		chat: func(context.Context, model.D) (model.ChatResponse, error) {
			chats++
			return model.ChatResponse{Choices: []model.Choice{responses[chats-1]}}, nil
		},
	}
	trainer := GoMLX{acquire: func(context.Context, TrainingJob, string) (inferenceModel, func() error, error) {
		return stub, func() error { return nil }, nil
	}}
	answers, err := trainer.Predict(t.Context(), TrainingJob{PredictionTokens: 192}, "", []Record{{Prompt: "one"}, {Prompt: "two"}, {Prompt: "three"}}, nil)
	if err != nil || !reflect.DeepEqual(answers, []string{"The answer runs on and", " \n\t", "no finish reason"}) {
		t.Fatalf("Predict: got %q, %v, want every answer as given", answers, err)
	}
}

func TestGoMLXPredictFailureCleanup(t *testing.T) {
	backendErr, releaseErr := errors.New("backend failed"), errors.New("release failed")
	choice := model.Choice{Message: &model.ResponseMessage{Content: "answer"}, FinishReasonPtr: new(model.FinishReasonStop)}
	tests := []struct {
		name     string
		choices  []model.Choice
		chatErr  error
		tokenErr error
		acqErr   error
	}{
		{name: "no choices"},
		{name: "multiple choices", choices: []model.Choice{choice, choice}},
		{name: "nil message", choices: []model.Choice{{FinishReasonPtr: new(model.FinishReasonStop)}}},
		{name: "backend", chatErr: backendErr},
		{name: "tokenizer", tokenErr: backendErr},
		{name: "acquire", acqErr: backendErr},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			released, chats := 0, 0
			stub := inferenceStub{
				cfg: model.Config{PtrContextWindow: new(512)},
				tokenize: func(context.Context, model.D) (model.TokenizeResponse, error) {
					return model.TokenizeResponse{Tokens: 1}, tt.tokenErr
				},
				chat: func(context.Context, model.D) (model.ChatResponse, error) {
					chats++
					if chats == 1 {
						return model.ChatResponse{Choices: []model.Choice{choice}}, nil
					}
					return model.ChatResponse{Choices: tt.choices}, tt.chatErr
				},
			}
			trainer := GoMLX{acquire: func(context.Context, TrainingJob, string) (inferenceModel, func() error, error) {
				if tt.acqErr != nil {
					return nil, nil, tt.acqErr
				}
				return stub, func() error { released++; return releaseErr }, nil
			}}
			answers, err := trainer.Predict(t.Context(), TrainingJob{PredictionTokens: 192}, "", []Record{{Prompt: "one"}, {Prompt: "two"}, {Prompt: "three"}}, nil)
			if err == nil || answers != nil {
				t.Fatalf("Predict: got %q, %v, want nil answers and error", answers, err)
			}
			wantReleased, wantChats := 1, 2
			if tt.acqErr != nil {
				wantReleased, wantChats = 0, 0
			} else if !errors.Is(err, releaseErr) {
				t.Errorf("error: got %v, want release failure joined", err)
			}
			if tt.tokenErr != nil {
				wantChats = 0
			}
			if (tt.chatErr != nil || tt.tokenErr != nil || tt.acqErr != nil) && !errors.Is(err, backendErr) {
				t.Errorf("error: got %v, want backend failure preserved", err)
			}
			if tt.chatErr == nil && tt.tokenErr == nil && tt.acqErr == nil && !strings.Contains(err.Error(), "prediction is missing") {
				t.Errorf("error: got %v, want prediction rejection as well as release failure", err)
			}
			if released != wantReleased || chats != wantChats {
				t.Errorf("release/chat calls: got %d/%d, want %d/%d", released, chats, wantReleased, wantChats)
			}
		})
	}
}

func TestGoMLXValidateContextBoundaries(t *testing.T) {
	for _, split := range []string{"training", "validation", "protected test"} {
		for _, count := range []int{255, 256, 257} {
			t.Run(fmt.Sprintf("%s/%d", split, count), func(t *testing.T) {
				ctx := t.Context()
				record := Record{Prompt: " question \n", Target: " SECRET TARGET \t"}
				dataset := TrainingDataset{}
				switch split {
				case "training":
					dataset.Train = []Record{record}
				case "validation":
					dataset.Validation = []Record{record}
				default:
					dataset.Test = []Record{record}
				}
				calls, released := 0, 0
				stub := inferenceStub{
					cfg: model.Config{PtrContextWindow: new(256)},
					ids: func(gotCtx context.Context, text string) ([]int32, error) {
						calls++
						want := "<bos><start_of_turn>user\nquestion<end_of_turn>\n<start_of_turn>model\nSECRET TARGET<end_of_turn>"
						if split == "protected test" || gotCtx != ctx || text != want {
							t.Errorf("TokenizeIDs: got %q, want %q only for training/validation with original context", text, want)
						}
						// Causal training consumes all but the final token as input.
						return make([]int32, count+1), nil
					},
					tokenize: func(gotCtx context.Context, input model.D) (model.TokenizeResponse, error) {
						calls++
						want := model.D{"input": record.Prompt, "apply_template": true}
						if split != "protected test" || gotCtx != ctx || !reflect.DeepEqual(input, want) {
							t.Errorf("Tokenize: got %#v, want %#v only for protected tests with original context", input, want)
						}
						return model.TokenizeResponse{Tokens: count - 192}, nil
					},
				}
				trainer := GoMLX{acquire: func(gotCtx context.Context, job TrainingJob, adapter string) (inferenceModel, func() error, error) {
					if gotCtx != ctx || adapter != "" || !reflect.DeepEqual(job.Dataset, dataset) {
						t.Errorf("acquire: got %v, %+v, %q, want original context/dataset and base adapter", gotCtx, job.Dataset, adapter)
					}
					return stub, func() error { released++; return nil }, nil
				}}
				err := trainer.Validate(ctx, TrainingJob{Dataset: dataset, PredictionTokens: 192})
				if count <= 256 && err != nil {
					t.Errorf("Validate: got %v, want nil at %d tokens", err, count)
				}
				if count > 256 && (!errors.Is(err, tmprl.ErrInvalidArgument) || !strings.Contains(err.Error(), split+" record 1")) {
					t.Errorf("Validate: got %v, want ErrInvalidArgument identifying %s record 1", err, split)
				}
				if calls != 1 || released != 1 {
					t.Errorf("tokenize/release calls: got %d/%d, want 1/1", calls, released)
				}
			})
		}
	}
}

func TestGoMLXPreflightFailureCleanup(t *testing.T) {
	backendErr, releaseErr := errors.New("tokenizer failed"), errors.New("release failed")
	for _, operation := range []string{"Validate", "Predict"} {
		for _, failure := range []string{"invalid window", "overflow", "tokenizer"} {
			t.Run(operation+"/"+failure, func(t *testing.T) {
				released := 0
				window := 256
				if failure == "invalid window" {
					window = 0
				}
				stub := inferenceStub{
					cfg: model.Config{PtrContextWindow: &window},
					chat: func(context.Context, model.D) (model.ChatResponse, error) {
						t.Fatal("Chat called after failed preflight")
						return model.ChatResponse{}, nil
					},
					tokenize: func(context.Context, model.D) (model.TokenizeResponse, error) {
						if failure == "tokenizer" {
							return model.TokenizeResponse{}, backendErr
						}
						return model.TokenizeResponse{Tokens: 65}, nil
					},
				}
				trainer := GoMLX{acquire: func(context.Context, TrainingJob, string) (inferenceModel, func() error, error) {
					return stub, func() error { released++; return releaseErr }, nil
				}}
				records := []Record{{Prompt: "question", Target: "secret"}}
				var err error
				if operation == "Validate" {
					err = trainer.Validate(t.Context(), TrainingJob{Dataset: TrainingDataset{Test: records}, PredictionTokens: 192})
				} else {
					var answers []string
					answers, err = trainer.Predict(t.Context(), TrainingJob{PredictionTokens: 192}, "", records, nil)
					if answers != nil {
						t.Errorf("Predict: got %q, want nil answers", answers)
					}
				}
				if released != 1 || !errors.Is(err, releaseErr) {
					t.Fatalf("cleanup: got %d releases, %v, want 1 release and joined release error", released, err)
				}
				switch failure {
				case "tokenizer":
					if !errors.Is(err, backendErr) {
						t.Errorf("error: got %v, want tokenizer failure preserved", err)
					}
				case "overflow":
					if !errors.Is(err, tmprl.ErrInvalidArgument) {
						t.Errorf("error: got %v, want ErrInvalidArgument", err)
					}
				default:
					if !strings.Contains(err.Error(), "invalid context window") {
						t.Errorf("error: got %v, want invalid context window", err)
					}
				}
			})
		}
	}
}

func TestGoMLXPredictionBudget(t *testing.T) {
	for _, budget := range []int{-1, 0, 63, 64, 65} {
		t.Run(fmt.Sprintf("budget=%d", budget), func(t *testing.T) {
			acquired, released, chats := 0, 0, 0
			stub := inferenceStub{
				cfg: model.Config{PtrContextWindow: new(256)},
				tokenize: func(context.Context, model.D) (model.TokenizeResponse, error) {
					return model.TokenizeResponse{Tokens: 192}, nil
				},
				chat: func(_ context.Context, input model.D) (model.ChatResponse, error) {
					chats++
					if input["max_completion_tokens"] != budget {
						t.Errorf("max_completion_tokens: got %v, want %d", input["max_completion_tokens"], budget)
					}
					return model.ChatResponse{Choices: []model.Choice{{Message: &model.ResponseMessage{Content: "answer"}, FinishReasonPtr: new(model.FinishReasonStop)}}}, nil
				},
			}
			trainer := GoMLX{acquire: func(context.Context, TrainingJob, string) (inferenceModel, func() error, error) {
				acquired++
				return stub, func() error { released++; return nil }, nil
			}}
			job := TrainingJob{PredictionTokens: budget, Dataset: TrainingDataset{Test: []Record{{Prompt: "question"}}}}
			err := trainer.Validate(t.Context(), job)
			wantErr := budget < 1 || budget > 64
			if (err != nil) != wantErr || (wantErr && !errors.Is(err, tmprl.ErrInvalidArgument)) {
				t.Errorf("Validate: got %v, want invalid argument=%v", err, wantErr)
			}
			answers, err := trainer.Predict(t.Context(), job, "", job.Dataset.Test, nil)
			if (err != nil) != wantErr || (wantErr && !errors.Is(err, tmprl.ErrInvalidArgument)) {
				t.Errorf("Predict: got %q, %v, want invalid argument=%v", answers, err, wantErr)
			}
			wantAcquired, wantChats := 2, 1
			if budget < 1 {
				wantAcquired = 1
			}
			if wantErr {
				wantChats = 0
			}
			if acquired != wantAcquired || released != wantAcquired || chats != wantChats {
				t.Errorf("acquire/release/chat: got %d/%d/%d, want %d/%d/%d", acquired, released, chats, wantAcquired, wantAcquired, wantChats)
			}
		})
	}
}

func TestGoMLXRequiresPool(t *testing.T) {
	provider, err := NewGoMLX(nil)
	if provider != nil || !errors.Is(err, tmprl.ErrInvalidArgument) {
		t.Errorf("NewGoMLX: got %v, %v, want nil, ErrInvalidArgument", provider, err)
	}
}

func TestKMSTokenizerExactIDs(t *testing.T) {
	ctx := t.Context()
	want := []int32{2, 0, 938, -1, 938}
	backendErr := errors.New("tokenizer failed")
	for _, wantErr := range []error{nil, backendErr} {
		tokenizer := kmsTokenizer{model: inferenceStub{ids: func(gotCtx context.Context, text string) ([]int32, error) {
			if gotCtx != ctx || text != "<bos> exact \n" {
				t.Errorf("TokenizeIDs: got %v, %q, want original context and exact text", gotCtx, text)
			}
			return want, wantErr
		}}}
		got, err := tokenizer.Tokenize(ctx, "<bos> exact \n")
		if !reflect.DeepEqual(got, want) || !errors.Is(err, wantErr) {
			t.Errorf("Tokenize: got %v, %v, want %v, %v", got, err, want, wantErr)
		}
	}
}

func TestKMSTokenizerVerifyModel(t *testing.T) {
	dir := t.TempDir()
	path, other, alias := filepath.Join(dir, "model.gguf"), filepath.Join(dir, "other.gguf"), filepath.Join(dir, "alias.gguf")
	for _, name := range []string{path, other} {
		if err := os.WriteFile(name, []byte("same bytes do not imply same file"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(path, alias); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	tests := []struct {
		name    string
		ctx     context.Context
		wanted  string
		files   []string
		wantErr error
	}{
		{name: "same file", wanted: path, files: []string{path}},
		{name: "symlink identity", wanted: alias, files: []string{path}},
		{name: "equal bytes different identity", wanted: path, files: []string{other}, wantErr: tmprl.ErrInvalidArgument},
		{name: "no files", wanted: path, wantErr: tmprl.ErrInvalidArgument},
		{name: "multiple files", wanted: path, files: []string{path, other}, wantErr: tmprl.ErrInvalidArgument},
		{name: "missing training model", wanted: filepath.Join(dir, "missing"), files: []string{path}, wantErr: os.ErrNotExist},
		{name: "missing served model", wanted: path, files: []string{filepath.Join(dir, "missing")}, wantErr: os.ErrNotExist},
		{name: "canceled before inspection", ctx: canceled, wanted: "missing", wantErr: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.ctx
			if ctx == nil {
				ctx = t.Context()
			}
			tokenizer := kmsTokenizer{model: inferenceStub{cfg: model.Config{ModelFiles: tt.files}}}
			if err := tokenizer.VerifyModel(ctx, tt.wanted); !errors.Is(err, tt.wantErr) {
				t.Errorf("VerifyModel: got %v, want %v", err, tt.wantErr)
			}
		})
	}
}
