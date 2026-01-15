package chatapi_test

import (
	"net/http"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

func respNonStream200(t *testing.T, tokens map[string]string) []apitest.Table {
	image, err := readFile(imageFile)
	if err != nil {
		t.Fatalf("read image: %s", err)
	}

	audio, err := readFile(audioFile)
	if err != nil {
		t.Fatalf("read audio: %s", err)
	}

	table := []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/responses",
			Token:      tokens["responses"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"input": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &kronk.ResponseResponse{},
			ExpResp: &kronk.ResponseResponse{
				Object: "response",
				Status: "completed",
				Model:  "Qwen3-8B-Q8_0",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(kronk.ResponseResponse{}, "ID", "CreatedAt", "CompletedAt", "Usage", "Output", "Temperature", "TopP", "ToolChoice", "Truncation", "Tools", "Metadata", "Text", "Reasoning", "ParallelToolCall", "Store"),
				)

				if diff != "" {
					return diff
				}

				return validateRespResponse(got).
					hasValidID().
					hasCreatedAt().
					hasStatus("completed").
					hasOutput().
					hasOutputText().
					warnContainsInContent("gorilla").
					result(t)
			},
		},
		{
			Name:       "image-good-token",
			URL:        "/v1/responses",
			Token:      tokens["responses"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":       "Qwen2.5-VL-3B-Instruct-Q8_0",
				"input":       model.ImageMessage("what's in the picture", image, "jpg"),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &kronk.ResponseResponse{},
			ExpResp: &kronk.ResponseResponse{
				Object: "response",
				Status: "completed",
				Model:  "Qwen2.5-VL-3B-Instruct-Q8_0",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(kronk.ResponseResponse{}, "ID", "CreatedAt", "CompletedAt", "Usage", "Output", "Temperature", "TopP", "ToolChoice", "Truncation", "Tools", "Metadata", "Text", "Reasoning", "ParallelToolCall", "Store"),
				)

				if diff != "" {
					return diff
				}

				return validateRespResponse(got).
					hasValidID().
					hasCreatedAt().
					hasStatus("completed").
					hasOutput().
					hasOutputText().
					warnContainsInContent("giraffes").
					result(t)
			},
		},
		{
			Name:       "audio-good-token",
			SkipInGH:   true,
			URL:        "/v1/responses",
			Token:      tokens["responses"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":       "Qwen2-Audio-7B.Q8_0",
				"input":       model.AudioMessage("please describe if you hear speech or not in this clip.", audio, "wav"),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &kronk.ResponseResponse{},
			ExpResp: &kronk.ResponseResponse{
				Object: "response",
				Status: "completed",
				Model:  "Qwen2-Audio-7B.Q8_0",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(kronk.ResponseResponse{}, "ID", "CreatedAt", "CompletedAt", "Usage", "Output", "Temperature", "TopP", "ToolChoice", "Truncation", "Tools", "Metadata", "Text", "Reasoning", "ParallelToolCall", "Store"),
				)

				if diff != "" {
					return diff
				}

				return validateRespResponse(got).
					hasValidID().
					hasCreatedAt().
					hasStatus("completed").
					hasOutput().
					hasOutputText().
					warnContainsInContent("speech").
					result(t)
			},
		},
	}

	return table
}

func respStream200(t *testing.T, tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/responses",
			Token:      tokens["responses"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"input": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
				"stream":      true,
			},
			GotResp: &kronk.ResponseStreamEvent{},
			ExpResp: &kronk.ResponseResponse{
				Object: "response",
				Status: "completed",
				Model:  "Qwen3-8B-Q8_0",
			},
			CmpFunc: func(got any, exp any) string {
				event := got.(*kronk.ResponseStreamEvent)
				if event.Response == nil {
					return "expected response.completed event with Response field"
				}

				diff := cmp.Diff(event.Response, exp,
					cmpopts.IgnoreFields(kronk.ResponseResponse{}, "ID", "CreatedAt", "CompletedAt", "Usage", "Output", "Temperature", "TopP", "ToolChoice", "Truncation", "Tools", "Metadata", "Text", "Reasoning", "ParallelToolCall", "Store"),
				)

				if diff != "" {
					return diff
				}

				return validateRespResponse(event.Response).
					hasValidID().
					hasCreatedAt().
					hasStatus("completed").
					hasOutput().
					hasOutputText().
					warnContainsInContent("gorilla").
					result(t)
			},
		},
	}

	return table
}

// =============================================================================

func respEndpoint401(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/responses",
			Token:      tokens["embeddings"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"input": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"responses\" not authorized",
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
			URL:        "/v1/responses",
			Token:      tokens["admin"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"input": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"responses\" not authorized",
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
