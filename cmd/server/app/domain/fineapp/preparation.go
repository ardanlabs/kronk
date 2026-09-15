package fineapp

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
)

// NewPreparation constructs raw-document extraction and model-backed dataset
// building. Docling is expected to be running before a raw upload begins.
func NewPreparation(modelPool *pool.Pool) (DocumentPreparer, DatasetBuilder, error) {
	if modelPool == nil {
		return nil, nil, fmt.Errorf("new fine-tuning preparation: model pool is required")
	}

	extractor, err := prepare.NewDoclingExtractor("", nil)
	if err != nil {
		return nil, nil, fmt.Errorf("new fine-tuning preparation: %w", err)
	}
	documents, err := prepare.New(extractor)
	if err != nil {
		return nil, nil, fmt.Errorf("new fine-tuning preparation: %w", err)
	}
	previews, err := prepare.NewPreviewer(newModelCompleter(modelPool))
	if err != nil {
		return nil, nil, fmt.Errorf("new fine-tuning preparation: %w", err)
	}

	return documents, previews, nil
}

type modelCompleter struct {
	chat func(context.Context, string, model.D) (model.ChatResponse, error)
}

func newModelCompleter(modelPool *pool.Pool) modelCompleter {
	return modelCompleter{
		chat: func(ctx context.Context, modelID string, request model.D) (model.ChatResponse, error) {
			krn, err := modelPool.AquireModel(ctx, modelID)
			if err != nil {
				return model.ChatResponse{}, fmt.Errorf("acquire preparation model: %w", err)
			}
			return krn.Chat(ctx, request)
		},
	}
}

func (c modelCompleter) Complete(ctx context.Context, request prepare.CompletionRequest) (prepare.Completion, error) {
	messages := make([]model.D, len(request.Messages))
	for index, message := range request.Messages {
		messages[index] = model.D{"role": message.Role, "content": message.Content}
	}

	response, err := c.chat(ctx, request.Model, model.D{
		"messages":              messages,
		"max_completion_tokens": request.MaxTokens,
		"temperature":           0.2,
		"top_p":                 0.9,
		"seed":                  request.Seed,
		"enable_thinking":       false,
		"response_format": model.D{
			"type": "json_schema",
			"json_schema": model.D{
				"name":   "moxie_training_records",
				"strict": true,
				"schema": request.ResponseSchema,
			},
		},
	})
	if err != nil {
		return prepare.Completion{}, fmt.Errorf("complete dataset preparation: %w", err)
	}
	if len(response.Choices) == 0 || response.Choices[0].Message == nil {
		return prepare.Completion{}, fmt.Errorf("complete dataset preparation: model returned no choices")
	}

	content := strings.TrimSpace(response.Choices[0].Message.Content)
	if content == "" {
		return prepare.Completion{}, fmt.Errorf("complete dataset preparation: model returned empty content")
	}

	completion := prepare.Completion{Content: content}
	if response.Usage != nil {
		completion.PromptTokens = response.Usage.PromptTokens
		completion.CompletionTokens = response.Usage.CompletionTokens
	}
	return completion, nil
}
