package chatapi_test

import (
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const (
	embeddingModelID   = "Qwen3-Embedding-0.6B-Q8_0"
	embeddingDimension = 1024
)

func chatEmbed200(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/embeddings",
			Token:      tokens["embeddings"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":       embeddingModelID,
				"input":       "Embed this sentence",
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &model.EmbedReponse{},
			ExpResp: &model.EmbedReponse{
				Model:  embeddingModelID,
				Object: "list",
				Data: []model.EmbedData{
					{
						Object: "embedding",
						Index:  0,
					},
				},
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.EmbedReponse{}, "Data", "Created", "Usage"),
					cmpopts.IgnoreFields(model.EmbedData{}, "Embedding"),
				)

				if diff != "" {
					return diff
				}

				expResp, ok := got.(*model.EmbedReponse)
				if !ok {
					return fmt.Sprintf("response wrong type: %T", got)
				}

				if len(expResp.Data) != 1 {
					return fmt.Sprintf("expected length of 1, got %d", len(expResp.Data))
				}

				if len(expResp.Data[0].Embedding) != embeddingDimension {
					return fmt.Sprintf("expecting a vector of %d dimensions", embeddingDimension)
				}

				if expResp.Usage.PromptTokens == 0 {
					return "expected prompt tokens to be non-zero"
				}

				return ""
			},
		},
		{
			Name:       "multi-good-token",
			URL:        "/v1/embeddings",
			Token:      tokens["embeddings"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":       embeddingModelID,
				"input":       []string{"Embed this sentence", "and this sentence"},
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &model.EmbedReponse{},
			ExpResp: &model.EmbedReponse{
				Model:  embeddingModelID,
				Object: "list",
				Data: []model.EmbedData{
					{
						Object: "embedding",
						Index:  0,
					},
				},
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.EmbedReponse{}, "Data", "Created", "Usage"),
					cmpopts.IgnoreFields(model.EmbedData{}, "Embedding"),
				)

				if diff != "" {
					return diff
				}

				expResp, ok := got.(*model.EmbedReponse)
				if !ok {
					return fmt.Sprintf("response wrong type: %T", got)
				}

				if len(expResp.Data) != 2 {
					return fmt.Sprintf("expected length of 2, got %d", len(expResp.Data))
				}

				if len(expResp.Data[0].Embedding) != embeddingDimension {
					return fmt.Sprintf("expecting a vector of %d dimensions", embeddingDimension)
				}

				if len(expResp.Data[1].Embedding) != embeddingDimension {
					return fmt.Sprintf("expecting a vector of %d dimensions", embeddingDimension)
				}

				if expResp.Usage.PromptTokens == 0 {
					return "expected prompt tokens to be non-zero"
				}

				return ""
			},
		},
	}

	return table
}

func embed403(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/embeddings",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusForbidden,
			Input: model.D{
				"model":       embeddingModelID,
				"input":       "Embed this sentence",
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
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
