package chatapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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

// chatNonStreamQwen3 returns chat tests for the Qwen3-1.7B model (text).
func chatNonStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				Object:            "chat.completion",
				SystemFingerprint: "fp_kronk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
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
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":   2048,
				"temperature":  0,
				"top_p":        0.9,
				"top_k":        40,
				"logprobs":     true,
				"top_logprobs": 3,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta", "Logprobs"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				validation := validateResponse(got, false).
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
				if validation != "" {
					return validation
				}

				resp := got.(*model.ChatResponse)
				for _, logprob := range resp.Choices[0].Logprobs.Content {
					if logprob.Token == "<|im_end|>" {
						return "expected logprobs.content to exclude the vocabulary EOG token"
					}
				}

				return ""
			},
		},
		{
			Name:       "good-token-logprobs-no-thinking",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":           2048,
				"temperature":          0,
				"top_p":                0.9,
				"top_k":                40,
				"logprobs":             true,
				"top_logprobs":         3,
				"chat_template_kwargs": model.D{"enable_thinking": false},
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta", "Logprobs"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				validation := validateResponse(got, false).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasUsage(false).
					hasContent().
					hasLogprobs(3).
					warnContainsInContent("gorilla").
					result(t)
				if validation != "" {
					return validation
				}

				resp := got.(*model.ChatResponse)
				if resp.Choices[0].Message.Reasoning != "" {
					return "expected reasoning to be empty when thinking is disabled"
				}
				if resp.Usage.CompletionTokensDetails.ReasoningTokens != 0 {
					return "expected reasoning_tokens to be zero when thinking is disabled"
				}

				var logprobBytes []byte
				for _, logprob := range resp.Choices[0].Logprobs.Content {
					if logprob.Token == "<|im_end|>" {
						return "expected logprobs.content to exclude the vocabulary EOG token"
					}
					logprobBytes = append(logprobBytes, logprob.Bytes...)
				}

				return cmp.Diff(resp.Choices[0].Message.Content, string(logprobBytes))
			},
		},
	}
}

// chatContextOverflowQwen3 verifies oversized text requests fail before a
// streaming or non-streaming response is committed.
func chatContextOverflowQwen3(tokens map[string]string) []apitest.Table {
	type errorResponse struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    string `json:"code"`
		} `json:"error"`
	}

	oversizedPrompt := strings.Repeat("the quick brown fox jumps over the lazy dog. ", 3400)

	table := func(name string, stream bool) apitest.Table {
		return apitest.Table{
			Name:       name,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusBadRequest,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, oversizedPrompt),
				),
				"max_tokens": 32,
				"stream":     stream,
			},
			GotResp: &errorResponse{},
			CmpFunc: func(got any, _ any) string {
				gotErr := got.(*errorResponse).Error
				if gotErr.Code != errs.InvalidArgument.String() {
					return fmt.Sprintf("error code: got %s, want %s", gotErr.Code, errs.InvalidArgument)
				}
				if gotErr.Type != "invalid_request_error" {
					return fmt.Sprintf("error type: got %s, want invalid_request_error", gotErr.Type)
				}
				if !strings.Contains(gotErr.Message, "input tokens") || !strings.Contains(gotErr.Message, "context window") {
					return fmt.Sprintf("error message %q does not describe the context overflow", gotErr.Message)
				}
				if strings.Contains(gotErr.Message, "imc decode") || strings.Contains(gotErr.Message, "extension tokens") {
					return fmt.Sprintf("error message exposes decoder internals: %q", gotErr.Message)
				}

				return ""
			},
		}
	}

	return []apitest.Table{
		table("non-streaming", false),
		table("streaming", true),
	}
}

// chatStreamQwen3 returns streaming chat tests for the Qwen3-1.7B model.
func chatStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
				"stream":      true,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			GotResp:              &model.ChatResponse{},
			StreamResponseOffset: 1,
			RequireDone:          true,
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasNoLogprobs().
					result(t)
			},
			StreamCmpFunc: validateChatUsageStream,
		},
		{
			Name:       "good-token-logprobs",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":   2048,
				"temperature":  0.7,
				"top_p":        0.9,
				"top_k":        40,
				"stream":       true,
				"logprobs":     true,
				"top_logprobs": 3,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			GotResp:              &model.ChatResponse{},
			StreamResponseOffset: 1,
			RequireDone:          true,
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
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
					hasNoLogprobs().
					result(t)
			},
			StreamCmpFunc: validateChatUsageStream,
		},
	}
}

// chatStreamIMCQwen3 returns streaming chat tests for IMC (Incremental Message Cache).
// These tests verify multi-turn caching behavior.
// Skipped in GitHub Actions as they require a model configured with IncrementalCache.
func chatStreamIMCQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "imc-first-turn",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleSystem, "You are a helpful assistant."),
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			GotResp:              &model.ChatResponse{},
			StreamResponseOffset: 1,
			RequireDone:          true,
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					result(t)
			},
			StreamCmpFunc: validateChatUsageStream,
		},
		{
			Name:       "imc-second-turn-cache-hit",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleSystem, "You are a helpful assistant."),
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
					model.TextMessage(model.RoleAssistant, "Gorilla"),
					model.TextMessage(model.RoleUser, "Now echo back the word: Elephant"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			GotResp:              &model.ChatResponse{},
			StreamResponseOffset: 1,
			RequireDone:          true,
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					result(t)
			},
			StreamCmpFunc: validateChatUsageStream,
		},
		{
			Name:       "imc-different-session",
			SkipInGH:   true,
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleSystem, "You are a helpful assistant."),
					model.TextMessage(model.RoleUser, "Echo back the word: Tiger"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"stream":      true,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			GotResp:              &model.ChatResponse{},
			StreamResponseOffset: 1,
			RequireDone:          true,
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					result(t)
			},
			StreamCmpFunc: validateChatUsageStream,
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
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessageArray(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
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
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessageArray(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
				"stream":      true,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			GotResp:              &model.ChatResponse{},
			StreamResponseOffset: 1,
			RequireDone:          true,
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasNoLogprobs().
					result(t)
			},
			StreamCmpFunc: validateChatUsageStream,
		},
	}
}

// chatImageQwen35 returns chat tests for the Qwen3.5 vision model.
func chatImageQwen35(t *testing.T, tokens map[string]string) []apitest.Table {
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
				"model":                visionModelID,
				"messages":             model.ImageMessage("what's in the picture", image, "jpg"),
				"max_tokens":           2048,
				"temperature":          0.7,
				"top_p":                0.9,
				"top_k":                40,
				"chat_template_kwargs": model.D{"enable_thinking": false},
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             visionModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.media",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
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
					warnContainsInContent("giraffes").
					result(t)
			},
		},
	}
}

// chatAudioQwen25Omni returns chat tests for Qwen2.5-Omni-3B-Q4_K_M model (audio).
func chatAudioQwen25Omni(t *testing.T, tokens map[string]string) []apitest.Table {
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
				"model":       "ggml-org/Qwen2.5-Omni-3B-Q4_K_M",
				"messages":    model.AudioMessage("please describe if you hear speech or not in this clip.", audio, "wav"),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             "ggml-org/Qwen2.5-Omni-3B-Q4_K_M",
				SystemFingerprint: "fp_kronk",
				Object:            "chat.media",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
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
					warnContainsInContent("speech").
					result(t)
			},
		},
	}
}

// chatGrammarQwen3 returns grammar-constrained chat tests for the Qwen3-1.7B model.
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
				"model": qwen3ModelID,
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
				Choices: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
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

// chatGrammarStreamQwen3 returns streaming grammar-constrained chat tests for the Qwen3-1.7B model.
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
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "List 3 programming languages with their year of creation. Respond in JSON format."),
				),
				"grammar":      grammarJSONObject,
				"temperature":  0.7,
				"max_tokens":   512,
				"stream":       true,
				"enable_think": false,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			GotResp:              &model.ChatResponse{},
			StreamResponseOffset: 1,
			RequireDone:          true,
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Message:         nil,
						FinishReasonPtr: new("stop"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReasonPtr", "Delta"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got, true).
					hasValidUUID().
					hasCreated().
					hasValidChoice().
					hasNoLogprobs().
					result(t)
			},
			StreamCmpFunc: func(events []json.RawMessage) string {
				return validateChatUsageStreamReasoning(events, false)
			},
		},
	}
}

// chatToolCallQwen3 returns tool call tests for the Qwen3-1.7B model.
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
				"model": qwen3ModelID,
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
				Choices: []model.Choice{
					{
						Message: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("tool_calls"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
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

// chatToolCallStreamQwen3 returns streaming tool call tests for the Qwen3-1.7B model.
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
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "What is the weather in NYC?"),
				),
				"tools":        tools,
				"stream":       true,
				"max_tokens":   512,
				"temperature":  0.7,
				"enable_think": true,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			GotResp:              &model.ChatResponse{},
			StreamResponseOffset: 1,
			RequireDone:          true,
			ExpResp: &model.ChatResponse{
				Choices: []model.Choice{
					{
						Delta: &model.ResponseMessage{
							Role: "assistant",
						},
						FinishReasonPtr: new("tool_calls"),
					},
				},
				Model:             qwen3ModelID,
				SystemFingerprint: "fp_kronk",
				Object:            "chat.completion.chunk",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage", "internal"),
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
					result(t)
			},
			StreamCmpFunc: func(events []json.RawMessage) string {
				if result := validateChatUsageStream(events); result != "" {
					return result
				}

				return validateToolCallStream(events[:len(events)-1], "get_weather", "location")
			},
		},
	}
}

// =============================================================================

func chatEndpoint403(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/chat/completions",
			Token:      tokens["embeddings"],
			Method:     http.MethodPost,
			StatusCode: http.StatusForbidden,
			Input: model.D{
				"model": qwen3ModelID,
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.PermissionDenied,
				Message: "rpc error: code = PermissionDenied desc = permission denied",
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
