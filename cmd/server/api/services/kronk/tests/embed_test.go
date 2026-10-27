package chatapi_test

import (
	"fmt"
	"net/http"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const (
	embeddingModelID   = "nomic-ai/nomic-embed-text-v1.5.Q8_0"
	embeddingDimension = 768
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

				if err := testlib.CheckEmbeddings(*expResp, embeddingModelID, 1, embeddingDimension); err != nil {
					return err.Error()
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

				if err := testlib.CheckEmbeddings(*expResp, embeddingModelID, 2, embeddingDimension); err != nil {
					return err.Error()
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
