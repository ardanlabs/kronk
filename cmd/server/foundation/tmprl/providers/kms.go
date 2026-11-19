package providers

import (
	"context"
	"fmt"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/foundation/tmprl"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/pool"
)

// KMSCompleter requests structured output through the host's KMS model pool.
type KMSCompleter struct {
	chat func(context.Context, string, model.D) (model.ChatResponse, int, error)
}

// NewKMSCompleter constructs a model-pool-backed structured completer.
// Preparation uses a separate pool entry with a 16K context window and IMC off,
// leaving normal inference settings unchanged.
// Temporal owns retries; each completion makes a single model request.
func NewKMSCompleter(modelPool *pool.Pool) (*KMSCompleter, error) {
	if modelPool == nil {
		return nil, fmt.Errorf("new KMS completer: %w: model pool is required", tmprl.ErrInvalidArgument)
	}

	c := KMSCompleter{
		chat: func(ctx context.Context, modelID string, request model.D) (model.ChatResponse, int, error) {
			cfg, err := modelPool.ResolvedKronkConfig(modelID)
			if err != nil {
				return model.ChatResponse{}, 0, fmt.Errorf("resolve completion model: %w", err)
			}
			cfg.PtrContextWindow = new(16384)
			cfg.PtrIncrementalCache = new(false)
			krn, err := modelPool.AquireCustom(ctx, modelID+"/custom/preparation", cfg)
			if err != nil {
				return model.ChatResponse{}, 0, fmt.Errorf("acquire completion model: %w", err)
			}
			response, err := krn.Chat(ctx, request)
			return response, krn.ModelConfig().ContextWindow(), err
		},
	}
	return &c, nil
}

// Complete requests structured content and reports usage without retrying.
func (c KMSCompleter) Complete(ctx context.Context, request CompletionRequest) (Completion, error) {
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
		"enable_thinking":       request.EnableThinking,
	}
	if request.ResponseSchema != nil {
		chatRequest["response_format"] = model.D{
			"type": "json_schema",
			"json_schema": model.D{
				"name":   "structured_output",
				"strict": true,
				"schema": request.ResponseSchema,
			},
		}
	}
	response, contextWindow, err := c.chat(ctx, request.Model, chatRequest)
	if err != nil {
		return Completion{}, fmt.Errorf("complete model request: %w", err)
	}
	if len(response.Choices) != 1 {
		return Completion{}, fmt.Errorf("complete model request: model must return exactly one choice")
	}
	choice := response.Choices[0]
	if choice.Message == nil {
		return Completion{}, fmt.Errorf("complete model request: model returned no message")
	}

	completion := Completion{Content: strings.TrimSpace(choice.Message.Content)}
	if response.Usage != nil {
		completion.PromptTokens = response.Usage.PromptTokens
		completion.CompletionTokens = response.Usage.CompletionTokens
	}
	switch choice.FinishReason() {
	case model.FinishReasonLength:
		return completion, fmt.Errorf("complete model request: %w after %d tokens", ErrCompletionTruncated, request.MaxTokens)
	case model.FinishReasonStop:
	default:
		return completion, fmt.Errorf("complete model request: model did not finish successfully")
	}
	if completion.Content == "" {
		return completion, fmt.Errorf("complete model request: model returned empty content")
	}
	if request.EnableThinking {
		switch {
		case response.Usage == nil:
			return completion, fmt.Errorf("complete model request: missing usage for context-budget validation")
		case response.Usage.PromptTokens > contextWindow-request.MaxTokens:
			return completion, fmt.Errorf("complete model request: insufficient context budget: prompt=%d, reserved_output=%d, context_window=%d", response.Usage.PromptTokens, request.MaxTokens, contextWindow)
		}
	}
	return completion, nil
}
