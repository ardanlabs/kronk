package chatapi_test

import (
	"net/http"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// =============================================================================
// Tests grouped by model to minimize model loading/unloading in CI.
// =============================================================================

// chatNonStreamQwen3 returns chat tests for Qwen3-8B-Q8_0 model (text).
func chatNonStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":    2048,
				"temperature":   0.7,
				"top_p":         0.9,
				"top_k":         40,
				"return_prompt": true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion",
				Prompt: "<|im_start|>user\nEcho back the word: Gorilla<|im_end|>\n<|im_start|>assistant\n",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, false).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					hasContent().
					hasReasoning().
					hasNoLogprobs().
					warnContainsInContent("gorilla").
					warnContainsInReasoning("gorilla").
					result(t)
			},
		},
		{
			Name:       "good-token-logprobs",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":    2048,
				"temperature":   0.7,
				"top_p":         0.9,
				"top_k":         40,
				"return_prompt": true,
				"logprobs":      true,
				"top_logprobs":  3,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion",
				Prompt: "<|im_start|>user\nEcho back the word: Gorilla<|im_end|>\n<|im_start|>assistant\n",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta", "Logprobs"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, false).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					hasContent().
					hasReasoning().
					hasLogprobs(3).
					warnContainsInContent("gorilla").
					warnContainsInReasoning("gorilla").
					result(t)
			},
		},
	}
}

// chatStreamQwen3 returns streaming chat tests for Qwen3-8B-Q8_0 model.
func chatStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":    2048,
				"temperature":   0.7,
				"top_p":         0.9,
				"top_k":         40,
				"stream":        true,
				"return_prompt": true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
				Prompt: "<|im_start|>user\nEcho back the word: Gorilla<|im_end|>\n<|im_start|>assistant\n",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					hasNoLogprobs().
					result(t)
			},
		},
		{
			Name:       "good-token-logprobs",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":    2048,
				"temperature":   0.7,
				"top_p":         0.9,
				"top_k":         40,
				"stream":        true,
				"return_prompt": true,
				"logprobs":      true,
				"top_logprobs":  3,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
				Prompt: "<|im_start|>user\nEcho back the word: Gorilla<|im_end|>\n<|im_start|>assistant\n",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta", "Logprobs"),
				)

				if diff != "" {
					return diff
				}

				// For streaming, logprobs are sent per-delta chunk, NOT in the final chunk.
				// The test framework only validates the final chunk, so we verify the final
				// chunk does NOT have accumulated logprobs (correct streaming behavior).
				// Per-delta logprobs validation would require a different test approach.
				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					hasNoLogprobs().
					result(t)
			},
		},
	}
}

// chatStreamIMCQwen3 returns streaming chat tests for IMC (Incremental Message Cache).
// These tests verify multi-turn caching behavior with the KRONK_CACHE_ID header.
// Skipped in GitHub Actions as they require a model configured with IncrementalCache.
func chatStreamIMCQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:     "imc-first-turn",
			SkipInGH: true,
			URL:      "/v1/chat/completions",
			Token:    tokens["chat-completions"],
			Headers: map[string]string{
				"KRONK_CACHE_ID": "test-session-1",
			},
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0/IMC",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleSystem, "You are a helpful assistant."),
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "Prompt"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					result(t)
			},
		},
		{
			Name:     "imc-second-turn-cache-hit",
			SkipInGH: true,
			URL:      "/v1/chat/completions",
			Token:    tokens["chat-completions"],
			Headers: map[string]string{
				"KRONK_CACHE_ID": "test-session-1",
			},
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0/IMC",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleSystem, "You are a helpful assistant."),
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
					model.TextMessage(model.RoleAssistant, "Gorilla"),
					model.TextMessage(model.RoleUser, "Now echo back the word: Elephant"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "Prompt"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					result(t)
			},
		},
		{
			Name:     "imc-different-session",
			SkipInGH: true,
			URL:      "/v1/chat/completions",
			Token:    tokens["chat-completions"],
			Headers: map[string]string{
				"KRONK_CACHE_ID": "test-session-2",
			},
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0/IMC",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleSystem, "You are a helpful assistant."),
					model.TextMessage(model.RoleUser, "Echo back the word: Tiger"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "Prompt"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					result(t)
			},
		},
	}
}

// chatStreamSPCQwen3 returns streaming chat tests for SPC (System Prompt Cache).
// These tests verify system prompt caching behavior.
// Skipped in GitHub Actions as they require a model configured with SystemPromptCache.
func chatStreamSPCQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "spc-first-request",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0/SPC",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleSystem, "You are a helpful assistant that always responds in exactly one word."),
					model.TextMessage(model.RoleUser, "What animal says meow?"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "Prompt"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					result(t)
			},
		},
		{
			Name:       "spc-cache-hit-same-system",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0/SPC",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleSystem, "You are a helpful assistant that always responds in exactly one word."),
					model.TextMessage(model.RoleUser, "What animal says woof?"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "Prompt"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					result(t)
			},
		},
		{
			Name:       "spc-no-system-uses-cache",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0/SPC",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "What animal says moo?"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "Prompt"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					result(t)
			},
		},
	}
}

// chatArrayFormatQwen3 returns chat tests using OpenAI array content format.
func chatArrayFormatQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "array-format-good-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessageArray(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":    2048,
				"temperature":   0.7,
				"top_p":         0.9,
				"top_k":         40,
				"return_prompt": true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion",
				Prompt: "<|im_start|>user\nEcho back the word: Gorilla<|im_end|>\n<|im_start|>assistant\n",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, false).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					hasContent().
					hasReasoning().
					hasNoLogprobs().
					warnContainsInContent("gorilla").
					warnContainsInReasoning("gorilla").
					result(t)
			},
		},
	}
}

// chatArrayFormatStreamQwen3 returns streaming chat tests using OpenAI array content format.
func chatArrayFormatStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "array-format-stream-good-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessageArray(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":    2048,
				"temperature":   0.7,
				"top_p":         0.9,
				"top_k":         40,
				"stream":        true,
				"return_prompt": true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
				Prompt: "<|im_start|>user\nEcho back the word: Gorilla<|im_end|>\n<|im_start|>assistant\n",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					hasNoLogprobs().
					result(t)
			},
		},
	}
}

// chatImageQwen25VL returns chat tests for Qwen2.5-VL-3B-Instruct-Q8_0 model (vision).
func chatImageQwen25VL(t *testing.T, tokens map[string]string) []apitest.Table {
	image, err := readFile(imageFile)
	if err != nil {
		t.Fatalf("read image: %s", err)
	}

	return []apitest.Table{
		{
			Name:       "image-good-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":       "Qwen2.5-VL-3B-Instruct-Q8_0",
				"messages":    model.ImageMessage("what's in the picture", image, "jpg"),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen2.5-VL-3B-Instruct-Q8_0",
				Object: "chat.media",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, false).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(false).
					hasContent().
					hasNoLogprobs().
					hasNoPrompt().
					warnContainsInContent("giraffes").
					result(t)
			},
		},
	}
}

// chatAudioQwen2Audio returns chat tests for Qwen2-Audio-7B.Q8_0 model (audio).
func chatAudioQwen2Audio(t *testing.T, tokens map[string]string) []apitest.Table {
	audio, err := readFile(audioFile)
	if err != nil {
		t.Fatalf("read audio: %s", err)
	}

	return []apitest.Table{
		{
			Name:       "audio-good-token",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":       "Qwen2-Audio-7B.Q8_0",
				"messages":    model.AudioMessage("please describe if you hear speech or not in this clip.", audio, "wav"),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen2-Audio-7B.Q8_0",
				Object: "chat.media",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, false).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(false).
					hasContent().
					hasNoLogprobs().
					hasNoPrompt().
					warnContainsInContent("speech").
					result(t)
			},
		},
	}
}

// chatGrammarQwen3 returns grammar-constrained chat tests for Qwen3-8B-Q8_0 model.
func chatGrammarQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "grammar-json",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "List 3 programming languages with their year of creation. Respond in JSON format."),
				),
				"grammar":      grammarJSONObject,
				"temperature":  0.7,
				"max_tokens":   512,
				"enable_think": false,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, false).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(false).
					hasContent().
					hasValidJSON().
					result(t)
			},
		},
	}
}

// chatGrammarStreamQwen3 returns streaming grammar-constrained chat tests for Qwen3-8B-Q8_0 model.
func chatGrammarStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "grammar-json-stream",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "List 3 programming languages with their year of creation. Respond in JSON format."),
				),
				"grammar":      grammarJSONObject,
				"temperature":  0.7,
				"max_tokens":   512,
				"stream":       true,
				"enable_think": false,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(false).
					hasNoLogprobs().
					result(t)
			},
		},
	}
}

// chatToolCallQwen3 returns tool call tests for Qwen3-8B-Q8_0 model.
func chatToolCallQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	tools := model.DocumentArray(
		model.D{
			"type": "function",
			"function": model.D{
				"name":        "get_weather",
				"description": "Get the current weather for a location",
				"parameters": model.D{
					"type": "object",
					"properties": model.D{
						"location": model.D{
							"type":        "string",
							"description": "The location to get the weather for, e.g. San Francisco, CA",
						},
					},
					"required": []any{"location"},
				},
			},
		},
	)

	return []apitest.Table{
		{
			Name:       "tool-call",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "What is the weather in NYC?"),
				),
				"tools":        tools,
				"max_tokens":   512,
				"temperature":  0.7,
				"enable_think": true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("tool_calls"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, false).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					hasToolCalls("get_weather").
					result(t)
			},
		},
	}
}

// chatToolCallStreamQwen3 returns streaming tool call tests for Qwen3-8B-Q8_0 model.
func chatToolCallStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	tools := model.DocumentArray(
		model.D{
			"type": "function",
			"function": model.D{
				"name":        "get_weather",
				"description": "Get the current weather for a location",
				"parameters": model.D{
					"type": "object",
					"properties": model.D{
						"location": model.D{
							"type":        "string",
							"description": "The location to get the weather for, e.g. San Francisco, CA",
						},
					},
					"required": []any{"location"},
				},
			},
		},
	)

	return []apitest.Table{
		{
			Name:       "tool-call-stream",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "What is the weather in NYC?"),
				),
				"tools":        tools,
				"stream":       true,
				"max_tokens":   512,
				"temperature":  0.7,
				"enable_think": true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Delta: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("tool_calls"),
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(true).
					hasToolCalls("get_weather").
					result(t)
			},
		},
	}
}

// =============================================================================

func chatEndpoint401(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["embeddings"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"chat-completions\" not authorized",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(errs.Error{}, "FuncName", "FileName"),
				)

				if diff != "" {
					return diff
				}

				return ""
			},
		},
		{
			Name:       "admin-only-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["admin"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"chat-completions\" not authorized",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(errs.Error{}, "FuncName", "FileName"),
				)

				if diff != "" {
					return diff
				}

				return ""
			},
		},
	}

	return table
}
