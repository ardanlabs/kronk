package chatapi_test

import (
	"net/http"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/security/auth"
	"github.com/ardanlabs/kronk/sdk/tools/security"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func Test_API(t *testing.T) {
	test := apitest.New(t, "Test_API")

	tokens := createTokens(t, test.Sec)

	test.Run(t, chatNonStream200(tokens), "chatnonstream-200")
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
				return cmp.Diff(got, exp,
					cmpopts.IgnoreUnexported(&model.ChatResponse{}, "ID", "Created", "Usage"),
					cmpopts.IgnoreUnexported(model.Choice{}, "Index", "FinishReason"),
					cmpopts.IgnoreUnexported(model.ResponseMessage{}, "Content", "Reasoning", "ToolCalls"),
				)
			},
		},
	}

	return table
}

// =============================================================================

func createTokens(t *testing.T, sec *security.Security) map[string]string {
	tokens := make(map[string]string)

	token, err := sec.GenerateToken(true, nil, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["admin"] = token

	// -------------------------------------------------------------------------

	token, err = sec.GenerateToken(true, nil, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["non-admin-no-endpoints"] = token

	// -------------------------------------------------------------------------

	endpoints := map[string]auth.RateLimit{
		"chat-completions": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["chat-completions"] = token

	// -------------------------------------------------------------------------

	endpoints = map[string]auth.RateLimit{
		"embeddings": {
			Limit:  0,
			Window: auth.RateUnlimited,
		},
	}

	token, err = sec.GenerateToken(false, endpoints, 60*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	tokens["embeddings"] = token

	return tokens
}
