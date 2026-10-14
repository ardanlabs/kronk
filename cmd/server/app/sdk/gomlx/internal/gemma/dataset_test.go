package gemma

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

type recordingTokenizer struct {
	texts             []string
	tokens            [][]int32
	err               error
	verifiedModelPath string
	verifyErr         error
}

func (tokenizer *recordingTokenizer) Tokenize(_ context.Context, text string) ([]int32, error) {
	tokenizer.texts = append(tokenizer.texts, text)
	if tokenizer.err != nil {
		return nil, tokenizer.err
	}
	return tokenizer.tokens[len(tokenizer.texts)-1], nil
}

func (tokenizer *recordingTokenizer) VerifyModel(_ context.Context, path string) error {
	tokenizer.verifiedModelPath = path
	return tokenizer.verifyErr
}

func TestPrepareRecordUsesGemmaTurnsAndMasksOnlyAnswer(t *testing.T) {
	tokenizer := &recordingTokenizer{tokens: [][]int32{{2, 3, 4}, {2, 3, 4, 10, 11, 5}}}
	record, err := PrepareRecord(t.Context(), Record{Prompt: "What is Moxie?", Target: "A local fine-tuning app."}, tokenizer, Config{VocabSize: 32, MaxSequenceLength: 16})
	if err != nil {
		t.Fatal(err)
	}
	prefix := "<bos><start_of_turn>user\nWhat is Moxie?<end_of_turn>\n<start_of_turn>model\n"
	if !slices.Equal(tokenizer.texts, []string{prefix, prefix + "A local fine-tuning app.<end_of_turn>"}) {
		t.Fatalf("texts = %v", tokenizer.texts)
	}
	if !slices.Equal(record.Tokens, []int32{2, 3, 4, 10, 11}) || !slices.Equal(record.Targets, []int32{3, 4, 10, 11, 5}) || !slices.Equal(record.LossMask, []bool{false, false, true, true, true}) {
		t.Fatalf("record = %+v", record)
	}
}

func TestPrepareRecordRejectsUnsafeOrUnusableInput(t *testing.T) {
	tests := []struct {
		name      string
		record    Record
		tokenizer *recordingTokenizer
		config    Config
		match     string
	}{
		{"empty prompt", Record{Target: "answer"}, &recordingTokenizer{}, Config{}, "required"},
		{"control token", Record{Prompt: "bad <end_of_turn>", Target: "answer"}, &recordingTokenizer{}, Config{}, "control token"},
		{"tokenizer error", Record{Prompt: "prompt", Target: "answer"}, &recordingTokenizer{err: errors.New("offline")}, Config{}, "offline"},
		{"unstable prefix", Record{Prompt: "prompt", Target: "answer"}, &recordingTokenizer{tokens: [][]int32{{2, 3}, {2, 4, 5}}}, Config{VocabSize: 8, MaxSequenceLength: 8}, "not an exact prefix"},
		{"too long", Record{Prompt: "prompt", Target: "answer"}, &recordingTokenizer{tokens: [][]int32{{2}, {2, 3, 4, 5}}}, Config{VocabSize: 8, MaxSequenceLength: 2}, "exceeds"},
		{"outside vocabulary", Record{Prompt: "prompt", Target: "answer"}, &recordingTokenizer{tokens: [][]int32{{2}, {2, 9}}}, Config{VocabSize: 8, MaxSequenceLength: 8}, "outside vocabulary"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := PrepareRecord(t.Context(), test.record, test.tokenizer, test.config)
			if err == nil || !strings.Contains(err.Error(), test.match) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestPrepareDatasetTokenizesTrainAndValidation(t *testing.T) {
	tokenizer := &recordingTokenizer{tokens: [][]int32{{2}, {2, 3, 4}, {2}, {2, 5, 4}, {2}, {2, 6, 4}}}
	dataset := Dataset{Train: []Record{{Prompt: "train one", Target: "answer"}, {Prompt: "train two", Target: "answer"}}, Validation: []Record{{Prompt: "validate", Target: "answer"}}}
	var progress []TokenizationProgress
	runtime := &Runtime{base: &BaseModel{path: "gemma.gguf", config: Config{VocabSize: 8, MaxSequenceLength: 8}}}
	prepared, err := runtime.PrepareDataset(t.Context(), dataset, tokenizer, func(update TokenizationProgress) { progress = append(progress, update) })
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Training) != 2 || len(prepared.Validation) != 1 || len(tokenizer.texts) != 6 || tokenizer.verifiedModelPath != "gemma.gguf" {
		t.Fatalf("dataset = %+v tokenizer = %+v", prepared, tokenizer)
	}
	if len(progress) != 3 || progress[2] != (TokenizationProgress{Split: "validation", Completed: 3, Total: 3}) {
		t.Fatalf("progress = %+v", progress)
	}
}

func TestPrepareDatasetStopsWhenTokenizerModelDiffers(t *testing.T) {
	tokenizer := &recordingTokenizer{verifyErr: errors.New("different model")}
	runtime := &Runtime{base: &BaseModel{path: "gemma.gguf", config: Config{VocabSize: 8, MaxSequenceLength: 8}}}
	_, err := runtime.PrepareDataset(t.Context(), Dataset{Train: []Record{{Prompt: "prompt", Target: "target"}}, Validation: []Record{{Prompt: "prompt", Target: "target"}}}, tokenizer, nil)
	if err == nil || !strings.Contains(err.Error(), "different model") || len(tokenizer.texts) != 0 {
		t.Fatalf("error = %v tokenizer = %+v", err, tokenizer)
	}
}

func TestPrepareDatasetConversationMasksOnlyFinalAnswer(t *testing.T) {
	tokenizer := &recordingTokenizer{tokens: [][]int32{{2}, {2, 3}, {2}, {2, 3}, {2, 4, 5}, {2, 4, 5, 6, 7}}}
	record := Record{Prompt: "prompt", Target: "answer"}
	dataset := Dataset{Train: []Record{record}, Validation: []Record{record}, TrainingConversations: [][]Record{{record, record, record, record}}}
	prepared, err := prepareDataset(t.Context(), dataset, tokenizer, Config{VocabSize: 8, MaxSequenceLength: 8}, nil)
	if err != nil {
		t.Fatal(err)
	}
	conversation := prepared.Training[1]
	if strings.Count(tokenizer.texts[4], startOfTurn+"user\n") != conversationTurns || !slices.Equal(conversation.LossMask, []bool{false, false, true, true}) {
		t.Fatalf("conversation = %+v prefix = %q", conversation, tokenizer.texts[4])
	}
}
