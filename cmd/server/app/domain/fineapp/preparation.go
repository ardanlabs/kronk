package fineapp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/fineapp/prepare"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
	"github.com/ardanlabs/kronk/sdk/tools/models"
)

const (
	preparationCompletionAttempts = 3
	preparationRetryDelay         = 250 * time.Millisecond
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
	chat       func(context.Context, string, model.D) (model.ChatResponse, error)
	retryDelay time.Duration
}

func newModelCompleter(modelPool *pool.Pool) modelCompleter {
	return modelCompleter{
		retryDelay: preparationRetryDelay,
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

	chatRequest := model.D{
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
	}
	var lastErr error
	attempts := 0
	for attempt := range preparationCompletionAttempts {
		attempts = attempt + 1
		response, err := c.chat(ctx, request.Model, chatRequest)
		if err == nil {
			if len(response.Choices) == 0 || response.Choices[0].Message == nil {
				err = errors.New("model returned no choices")
			} else if response.Choices[0].FinishReason() == model.FinishReasonLength {
				err = fmt.Errorf("%w after %d tokens", prepare.ErrCompletionTruncated, request.MaxTokens)
			} else if content := strings.TrimSpace(response.Choices[0].Message.Content); content == "" {
				err = errors.New("model returned empty content")
			} else {
				completion := prepare.Completion{Content: content}
				if response.Usage != nil {
					completion.PromptTokens = response.Usage.PromptTokens
					completion.CompletionTokens = response.Usage.CompletionTokens
				}
				return completion, nil
			}
		}
		lastErr = err
		if attempt == preparationCompletionAttempts-1 || !retryablePreparationCompletion(ctx, err) {
			break
		}
		timer := time.NewTimer(time.Duration(attempt+1) * c.retryDelay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return prepare.Completion{}, fmt.Errorf("complete dataset preparation: %w", ctx.Err())
		case <-timer.C:
		}
	}
	return prepare.Completion{}, fmt.Errorf("complete dataset preparation after %d attempts: %w", attempts, lastErr)
}

func retryablePreparationCompletion(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	switch {
	case errors.Is(err, prepare.ErrCompletionTruncated),
		errors.Is(err, model.ErrInvalidRequest),
		errors.Is(err, model.ErrMessagesMissing),
		errors.Is(err, model.ErrMessagesInvalid),
		errors.Is(err, model.ErrFileInputsUnsupported),
		errors.Is(err, pool.ErrNoCapacity),
		errors.Is(err, models.ErrInvalidModelID),
		errors.Is(err, models.ErrModelNotFound):
		return false
	case errors.Is(err, kronk.ErrAdmissionTimeout), errors.Is(err, pool.ErrServerBusy):
		return true
	default:
		return err != nil
	}
}
