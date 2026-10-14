package gemma

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

const (
	beginOfSequence   = "<bos>"
	startOfTurn       = "<start_of_turn>"
	endOfTurn         = "<end_of_turn>"
	conversationTurns = 4
)

// Record is a model-neutral prompt and target pair.
type Record struct {
	Prompt string
	Target string
}

// Dataset contains source splits and optional caller-selected conversations.
// Only the final turn of a conversation contributes to the loss.
type Dataset struct {
	Train                   []Record
	Validation              []Record
	TrainingConversations   [][]Record
	ValidationConversations [][]Record
}

// Tokenizer converts Gemma-formatted text into exact model token IDs.
type Tokenizer interface {
	Tokenize(context.Context, string) ([]int32, error)
}

// VerifiedTokenizer identifies the GGUF that supplies its token vocabulary.
type VerifiedTokenizer interface {
	Tokenizer
	VerifyModel(context.Context, string) error
}

// TrainingRecord is one answer-masked causal language-model sequence.
type TrainingRecord struct {
	Tokens   []int32
	Targets  []int32
	LossMask []bool
}

// PreparedDataset contains only the splits available during model selection.
// The protected test split is intentionally not tokenized here.
type PreparedDataset struct {
	Training   []TrainingRecord
	Validation []TrainingRecord
}

// TokenizationProgress reports completed source records across both splits.
type TokenizationProgress struct {
	Split     string
	Completed int
	Total     int
}

// PrepareDataset verifies the tokenizer's model identity before creating the
// model-specific training and validation records.
func (runtime *Runtime) PrepareDataset(ctx context.Context, contract Dataset, tokenizer VerifiedTokenizer, progress func(TokenizationProgress)) (PreparedDataset, error) {
	if runtime == nil || runtime.base == nil {
		return PreparedDataset{}, fmt.Errorf("gemma runtime is closed")
	}
	if tokenizer == nil {
		return PreparedDataset{}, fmt.Errorf("gemma tokenizer is required")
	}
	if err := tokenizer.VerifyModel(ctx, runtime.base.path); err != nil {
		return PreparedDataset{}, fmt.Errorf("verify Gemma tokenizer model: %w", err)
	}
	return prepareDataset(ctx, contract, tokenizer, runtime.base.config, progress)
}

func prepareDataset(ctx context.Context, contract Dataset, tokenizer Tokenizer, config Config, progress func(TokenizationProgress)) (PreparedDataset, error) {
	if len(contract.Train) == 0 {
		return PreparedDataset{}, fmt.Errorf("prepare Gemma dataset: training split is empty")
	}
	if len(contract.Validation) == 0 {
		return PreparedDataset{}, fmt.Errorf("prepare Gemma dataset: validation split is empty")
	}
	trainingConversations, validationConversations := contract.TrainingConversations, contract.ValidationConversations
	total := len(contract.Train) + len(contract.Validation) + len(trainingConversations) + len(validationConversations)
	completed := 0
	prepare := func(split string, sources []Record) ([]TrainingRecord, error) {
		records := make([]TrainingRecord, len(sources))
		for index, source := range sources {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("prepare Gemma dataset: %w", err)
			}
			record, err := PrepareRecord(ctx, source, tokenizer, config)
			if err != nil {
				return nil, fmt.Errorf("prepare Gemma dataset: %s record %d: %w", split, index+1, err)
			}
			records[index] = record
			completed++
			if progress != nil {
				progress(TokenizationProgress{Split: split, Completed: completed, Total: total})
			}
		}
		return records, nil
	}

	training, err := prepare("training", contract.Train)
	if err != nil {
		return PreparedDataset{}, err
	}
	validation, err := prepare("validation", contract.Validation)
	if err != nil {
		return PreparedDataset{}, err
	}
	prepareConversations := func(split string, sources [][]Record) ([]TrainingRecord, error) {
		records := make([]TrainingRecord, len(sources))
		for index, source := range sources {
			record, err := prepareConversationRecord(ctx, source, tokenizer, config)
			if err != nil {
				return nil, fmt.Errorf("prepare Gemma dataset: %s conversation %d: %w", split, index+1, err)
			}
			records[index] = record
			completed++
			if progress != nil {
				progress(TokenizationProgress{Split: split, Completed: completed, Total: total})
			}
		}
		return records, nil
	}
	conversationalTraining, err := prepareConversations("training", trainingConversations)
	if err != nil {
		return PreparedDataset{}, err
	}
	conversationalValidation, err := prepareConversations("validation", validationConversations)
	if err != nil {
		return PreparedDataset{}, err
	}
	training = append(training, conversationalTraining...)
	validation = append(validation, conversationalValidation...)
	return PreparedDataset{Training: training, Validation: validation}, nil
}

// PrepareRecord formats and tokenizes one model-neutral Moxie dataset record
// using Gemma's official user/model turn markers.
func PrepareRecord(ctx context.Context, source Record, tokenizer Tokenizer, config Config) (TrainingRecord, error) {
	return prepareConversationRecord(ctx, []Record{source}, tokenizer, config)
}

func prepareConversationRecord(ctx context.Context, sources []Record, tokenizer Tokenizer, config Config) (TrainingRecord, error) {
	if tokenizer == nil {
		return TrainingRecord{}, fmt.Errorf("gemma tokenizer is required")
	}
	if err := ctx.Err(); err != nil {
		return TrainingRecord{}, fmt.Errorf("prepare Gemma record: %w", err)
	}
	if len(sources) == 0 {
		return TrainingRecord{}, fmt.Errorf("prepare Gemma record: at least one conversation turn is required")
	}
	var prefix strings.Builder
	prefix.WriteString(beginOfSequence)
	for index, source := range sources {
		prompt := strings.TrimSpace(source.Prompt)
		target := strings.TrimSpace(source.Target)
		if prompt == "" || target == "" {
			return TrainingRecord{}, fmt.Errorf("prepare Gemma record: prompt and target are required")
		}
		for _, marker := range []string{beginOfSequence, startOfTurn, endOfTurn} {
			if strings.Contains(prompt, marker) || strings.Contains(target, marker) {
				return TrainingRecord{}, fmt.Errorf("prepare Gemma record: prompt and target must not contain model control token %q", marker)
			}
		}
		prefix.WriteString(startOfTurn + "user\n" + prompt + endOfTurn + "\n" + startOfTurn + "model\n")
		if index < len(sources)-1 {
			prefix.WriteString(target + endOfTurn + "\n")
		}
	}

	target := strings.TrimSpace(sources[len(sources)-1].Target)
	prefixText := prefix.String()
	full := prefixText + target + endOfTurn
	prefixTokens, err := tokenizer.Tokenize(ctx, prefixText)
	if err != nil {
		return TrainingRecord{}, fmt.Errorf("prepare Gemma record: tokenize prompt: %w", err)
	}
	fullTokens, err := tokenizer.Tokenize(ctx, full)
	if err != nil {
		return TrainingRecord{}, fmt.Errorf("prepare Gemma record: tokenize prompt and target: %w", err)
	}
	if len(prefixTokens) == 0 || len(fullTokens) <= len(prefixTokens) || !slices.Equal(prefixTokens, fullTokens[:min(len(prefixTokens), len(fullTokens))]) {
		return TrainingRecord{}, fmt.Errorf("prepare Gemma record: prompt token IDs are not an exact prefix of the complete record")
	}
	if len(fullTokens)-1 > config.MaxSequenceLength {
		return TrainingRecord{}, fmt.Errorf("prepare Gemma record: token count %d exceeds Gemma context length %d", len(fullTokens)-1, config.MaxSequenceLength)
	}
	for index, token := range fullTokens {
		if token < 0 || int(token) >= config.VocabSize {
			return TrainingRecord{}, fmt.Errorf("prepare Gemma record: token[%d]=%d is outside vocabulary [0,%d)", index, token, config.VocabSize)
		}
	}

	record := TrainingRecord{
		Tokens:   slices.Clone(fullTokens[:len(fullTokens)-1]),
		Targets:  slices.Clone(fullTokens[1:]),
		LossMask: make([]bool, len(fullTokens)-1),
	}
	for position := len(prefixTokens) - 1; position < len(record.LossMask); position++ {
		record.LossMask[position] = true
	}
	return record, nil
}
