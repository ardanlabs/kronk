package gemma

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/finetune"
)

const (
	beginOfSequence   = "<bos>"
	startOfTurn       = "<start_of_turn>"
	endOfTurn         = "<end_of_turn>"
	conversationTurns = 4
)

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
func (runtime *Runtime) PrepareDataset(ctx context.Context, contract finetune.DatasetContract, tokenizer VerifiedTokenizer, progress func(TokenizationProgress)) (PreparedDataset, error) {
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

func prepareDataset(ctx context.Context, contract finetune.DatasetContract, tokenizer Tokenizer, config Config, progress func(TokenizationProgress)) (PreparedDataset, error) {
	if len(contract.Train) == 0 {
		return PreparedDataset{}, fmt.Errorf("prepare Gemma dataset: training split is empty")
	}
	if len(contract.Validation) == 0 {
		return PreparedDataset{}, fmt.Errorf("prepare Gemma dataset: validation split is empty")
	}
	trainingConversations, validationConversations := conversationSources(contract.Train, contract.Validation)
	total := len(contract.Train) + len(contract.Validation) + len(trainingConversations) + len(validationConversations)
	completed := 0
	prepare := func(split string, sources []finetune.DatasetRecord) ([]TrainingRecord, error) {
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
	prepareConversations := func(split string, sources [][]finetune.DatasetRecord) ([]TrainingRecord, error) {
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

// conversationSources adds one four-turn example for every training fact and
// one per validation record. This teaches each fact to tolerate unrelated
// conversation history without a second training phase.
func conversationSources(training, validation []finetune.DatasetRecord) ([][]finetune.DatasetRecord, [][]finetune.DatasetRecord) {
	groups := make(map[string][]finetune.DatasetRecord)
	factOrder := make([]string, 0)
	for _, record := range training {
		if record.IndependentPrompt || record.FactID == "" {
			continue
		}
		if len(groups[record.FactID]) == 0 {
			factOrder = append(factOrder, record.FactID)
		}
		groups[record.FactID] = append(groups[record.FactID], record)
	}
	if len(factOrder) < conversationTurns {
		return nil, nil
	}
	representatives := make([]finetune.DatasetRecord, 0, len(factOrder))
	trainingSources := make([]finetune.DatasetRecord, 0, len(factOrder))
	for _, factID := range factOrder {
		records := groups[factID]
		representatives = append(representatives, records[0])
		trainingSources = append(trainingSources, records[len(records)-1])
	}
	trainingConversations := make([][]finetune.DatasetRecord, len(trainingSources))
	for index, source := range trainingSources {
		trainingConversations[index] = conversation(representatives, source, index+conversationTurns)
	}
	validationConversations := make([][]finetune.DatasetRecord, len(validation))
	for index, source := range validation {
		validationConversations[index] = conversation(representatives, source, index+conversationTurns)
	}
	return trainingConversations, validationConversations
}

func conversation(representatives []finetune.DatasetRecord, current finetune.DatasetRecord, offset int) []finetune.DatasetRecord {
	turns := make([]finetune.DatasetRecord, 0, conversationTurns)
	for step := 1; len(turns) < conversationTurns-1; step++ {
		candidate := representatives[(offset+step)%len(representatives)]
		if candidate.FactID != current.FactID {
			turns = append(turns, candidate)
		}
	}
	return append(turns, current)
}

// PrepareRecord formats and tokenizes one model-neutral Moxie dataset record
// using Gemma's official user/model turn markers.
func PrepareRecord(ctx context.Context, source finetune.DatasetRecord, tokenizer Tokenizer, config Config) (TrainingRecord, error) {
	return prepareConversationRecord(ctx, []finetune.DatasetRecord{source}, tokenizer, config)
}

func prepareConversationRecord(ctx context.Context, sources []finetune.DatasetRecord, tokenizer Tokenizer, config Config) (TrainingRecord, error) {
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
