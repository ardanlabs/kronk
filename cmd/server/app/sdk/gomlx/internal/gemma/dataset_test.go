package gemma

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
)

type recordingTokenizer struct {
	texts             []string
	tokens            [][]int32
	err               error
	verifiedModelPath string
	verifyErr         error
}

type textTokenizer struct {
	texts []string
}

func (tokenizer *textTokenizer) Tokenize(_ context.Context, text string) ([]int32, error) {
	tokenizer.texts = append(tokenizer.texts, text)
	tokens := make([]int32, len([]byte(text)))
	for index, value := range []byte(text) {
		tokens[index] = int32(value) + 1
	}
	return tokens, nil
}

func (tokenizer *recordingTokenizer) Tokenize(_ context.Context, text string) ([]int32, error) {
	tokenizer.texts = append(tokenizer.texts, text)
	if tokenizer.err != nil {
		return nil, tokenizer.err
	}
	return tokenizer.tokens[len(tokenizer.texts)-1], nil
}

func (tokenizer *recordingTokenizer) VerifyModel(_ context.Context, modelPath string) error {
	tokenizer.verifiedModelPath = modelPath
	return tokenizer.verifyErr
}

func TestPrepareRecordUsesGemmaTurnsAndMasksOnlyAnswer(t *testing.T) {
	tokenizer := &recordingTokenizer{tokens: [][]int32{
		{2, 3, 4},
		{2, 3, 4, 10, 11, 5},
	}}
	record, err := PrepareRecord(
		t.Context(),
		finetune.DatasetRecord{Prompt: "What is Moxie?", Target: "A local fine-tuning app."},
		tokenizer,
		Config{VocabSize: 32, MaxSequenceLength: 16},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantPrefix := "<bos><start_of_turn>user\nWhat is Moxie?<end_of_turn>\n<start_of_turn>model\n"
	if !slices.Equal(tokenizer.texts, []string{wantPrefix, wantPrefix + "A local fine-tuning app.<end_of_turn>"}) {
		t.Fatalf("tokenized text = %#v", tokenizer.texts)
	}
	if !slices.Equal(record.Tokens, []int32{2, 3, 4, 10, 11}) {
		t.Fatalf("tokens = %v", record.Tokens)
	}
	if !slices.Equal(record.Targets, []int32{3, 4, 10, 11, 5}) {
		t.Fatalf("targets = %v", record.Targets)
	}
	if !slices.Equal(record.LossMask, []bool{false, false, true, true, true}) {
		t.Fatalf("loss mask = %v", record.LossMask)
	}
}

func TestPrepareRecordRejectsUnsafeOrUnusableInput(t *testing.T) {
	tests := []struct {
		name      string
		record    finetune.DatasetRecord
		tokenizer *recordingTokenizer
		config    Config
		match     string
	}{
		{"empty prompt", finetune.DatasetRecord{Target: "answer"}, &recordingTokenizer{}, Config{}, "required"},
		{"control token", finetune.DatasetRecord{Prompt: "bad <end_of_turn>", Target: "answer"}, &recordingTokenizer{}, Config{}, "control token"},
		{"tokenizer error", finetune.DatasetRecord{Prompt: "prompt", Target: "answer"}, &recordingTokenizer{err: errors.New("offline")}, Config{}, "offline"},
		{"unstable prefix", finetune.DatasetRecord{Prompt: "prompt", Target: "answer"}, &recordingTokenizer{tokens: [][]int32{{2, 3}, {2, 4, 5}}}, Config{VocabSize: 8, MaxSequenceLength: 8}, "not an exact prefix"},
		{"too long", finetune.DatasetRecord{Prompt: "prompt", Target: "answer"}, &recordingTokenizer{tokens: [][]int32{{2}, {2, 3, 4, 5}}}, Config{VocabSize: 8, MaxSequenceLength: 2}, "exceeds"},
		{"outside vocabulary", finetune.DatasetRecord{Prompt: "prompt", Target: "answer"}, &recordingTokenizer{tokens: [][]int32{{2}, {2, 9}}}, Config{VocabSize: 8, MaxSequenceLength: 8}, "outside vocabulary"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := PrepareRecord(t.Context(), test.record, test.tokenizer, test.config)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v, want containing %q", err, test.match)
			}
		})
	}
}

func TestPrepareDatasetTokenizesTrainAndValidationButNotTest(t *testing.T) {
	tokenizer := &recordingTokenizer{tokens: [][]int32{
		{2}, {2, 3, 4},
		{2}, {2, 5, 4},
		{2}, {2, 6, 4},
	}}
	contract := finetune.DatasetContract{
		Train: []finetune.DatasetRecord{
			{Prompt: "train one", Target: "answer"},
			{Prompt: "train two", Target: "answer"},
		},
		Validation: []finetune.DatasetRecord{{Prompt: "validate", Target: "answer"}},
		Test:       []finetune.DatasetRecord{{Prompt: "<end_of_turn>", Target: "must stay protected"}},
	}
	var progress []TokenizationProgress
	runtime := &Runtime{base: &BaseModel{path: "gemma.gguf", config: Config{VocabSize: 8, MaxSequenceLength: 8}}}
	prepared, err := runtime.PrepareDataset(t.Context(), contract, tokenizer, func(update TokenizationProgress) {
		progress = append(progress, update)
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Training) != 2 || len(prepared.Validation) != 1 {
		t.Fatalf("prepared dataset = %+v", prepared)
	}
	if len(tokenizer.texts) != 6 {
		t.Fatalf("tokenizer calls = %d, want 6 without protected test", len(tokenizer.texts))
	}
	if tokenizer.verifiedModelPath != "gemma.gguf" {
		t.Fatalf("verified model = %q", tokenizer.verifiedModelPath)
	}
	if len(progress) != 3 || progress[2] != (TokenizationProgress{Split: "validation", Completed: 3, Total: 3}) {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestPrepareDatasetStopsWhenTokenizerModelDiffers(t *testing.T) {
	tokenizer := &recordingTokenizer{verifyErr: errors.New("different model")}
	runtime := &Runtime{base: &BaseModel{path: "gemma.gguf", config: Config{VocabSize: 8, MaxSequenceLength: 8}}}
	_, err := runtime.PrepareDataset(t.Context(), finetune.DatasetContract{
		Train:      []finetune.DatasetRecord{{Prompt: "prompt", Target: "target"}},
		Validation: []finetune.DatasetRecord{{Prompt: "prompt", Target: "target"}},
	}, tokenizer, nil)
	if err == nil || !strings.Contains(err.Error(), "different model") {
		t.Fatalf("error = %v", err)
	}
	if len(tokenizer.texts) != 0 {
		t.Fatal("dataset was tokenized before model identity was verified")
	}
}

func TestPrepareDatasetBalancesSingleAndConversationRecordsWithoutProtectedTest(t *testing.T) {
	training := make([]finetune.DatasetRecord, 0, 30)
	for fact := range 10 {
		for variant := range 3 {
			training = append(training, finetune.DatasetRecord{
				Prompt: fmt.Sprintf("Training question %d variant %d?", fact+1, variant+1),
				Target: fmt.Sprintf("Training answer %d", fact+1), FactID: fmt.Sprintf("fact-%d", fact+1),
			})
		}
	}
	contract := finetune.DatasetContract{
		Train: training,
		Validation: []finetune.DatasetRecord{
			{Prompt: "Validation question?", Target: "Validation answer", FactID: "fact-1", IndependentPrompt: true},
		},
		Test: []finetune.DatasetRecord{
			{Prompt: "Protected question must remain hidden?", Target: "Protected answer", FactID: "protected", IndependentPrompt: true},
		},
	}
	tokenizer := &textTokenizer{}
	prepared, err := prepareDataset(t.Context(), contract, tokenizer, Config{VocabSize: 257, MaxSequenceLength: 4096}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Training) != 40 || len(prepared.Validation) != 2 {
		t.Fatalf("balanced dataset = training %d validation %d", len(prepared.Training), len(prepared.Validation))
	}
	for _, text := range tokenizer.texts {
		if strings.Contains(text, "Protected question") || strings.Contains(text, "Protected answer") {
			t.Fatalf("protected test leaked into conversational training: %q", text)
		}
	}
	conversationOffset := len(contract.Train) + len(contract.Validation)
	coveredFacts := make(map[string]bool, 10)
	for index := range len(prepared.Training) - len(contract.Train) {
		prefix := tokenizer.texts[(conversationOffset+index)*2]
		if turns := strings.Count(prefix, startOfTurn+"user\n"); turns != conversationTurns {
			t.Fatalf("conversation %d has %d turns", index+1, turns)
		}
		for fact := range 10 {
			question := fmt.Sprintf("Training question %d variant 3?", fact+1)
			lastTurn := startOfTurn + "user\n" + question + endOfTurn + "\n" + startOfTurn + "model\n"
			if strings.HasSuffix(prefix, lastTurn) {
				coveredFacts[question] = true
			}
		}
	}
	if len(coveredFacts) != 10 {
		t.Fatalf("conversation fact coverage = %d, want 10", len(coveredFacts))
	}
	lastPrefix := tokenizer.texts[len(tokenizer.texts)-2]
	if !strings.Contains(lastPrefix, "Validation question?") {
		t.Fatalf("validation conversation prefix = %q", lastPrefix)
	}
}
