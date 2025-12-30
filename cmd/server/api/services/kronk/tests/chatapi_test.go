package chatapi_test

import (
	"net/http"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func Test_API(t *testing.T) {
	test := apitest.New(t, "Test_API")

	tokens := createTokens(t, test.Sec)

	test.Run(t, chatNonStream200(tokens), "chatnonstream-200")
	test.RunStreaming(t, chatStream200(tokens), "chatstream-200")
}

func chatNonStream200(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "basic",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
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
				Choice: []model.Choice{
					{
						Delta: model.ResponseMessage{
							Role: "assistant",
						},
						FinishReason: "stop",
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
				Prompt: "<|im_start|>user\nEcho back the word: Gorilla<|im_end|>\n<|im_start|>assistant\n",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReason"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got).
					hasValidUUID().
					hasCreated().
					hasPrompt().
					hasValidChoice().
					hasUsage().
					hasContentOrReasoning().
					containsInContent("gorilla").
					containsInReasoning("gorilla").
					result()
			},
		},
	}

	return table
}

func chatStream200(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "basic",
			URL:        "/v1/chat/completions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"messages": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":         2048,
				"temperature":        0.7,
				"top_p":              0.9,
				"top_k":              40,
				"stream":             true,
				"keep_final_content": true,
			},
			GotResp: &model.ChatResponse{},
			ExpResp: &model.ChatResponse{
				Choice: []model.Choice{
					{
						Delta: model.ResponseMessage{
							Role: "assistant",
						},
						FinishReason: "stop",
					},
				},
				Model:  "Qwen3-8B-Q8_0",
				Object: "chat.completion.chunk",
				Prompt: "<|im_start|>user\nEcho back the word: Gorilla<|im_end|>\n<|im_start|>assistant\n",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreFields(model.Choice{}, "Index", "FinishReason"),
					cmpopts.IgnoreFields(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)

				if diff != "" {
					return diff
				}

				return validateResponse(got).
					hasValidUUID().
					hasCreated().
					hasPrompt().
					hasValidChoice().
					hasUsage().
					hasContentOrReasoning().
					containsInContent("gorilla").
					containsInReasoning("gorilla").
					result()
			},
		},
	}

	return table
}
