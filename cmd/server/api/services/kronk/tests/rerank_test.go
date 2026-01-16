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

func rerank200(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/rerank",
			Token:      tokens["rerank"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "bge-reranker-v2-m3-Q8_0",
				"query": "What is the capital of France?",
				"documents": []string{
					"Paris is the capital and largest city of France.",
					"Berlin is the capital of Germany.",
					"The Eiffel Tower is located in Paris.",
				},
				"top_n":            2,
				"return_documents": true,
			},
			GotResp: &model.RerankResponse{},
			ExpResp: &model.RerankResponse{
				Model:  "bge-reranker-v2-m3-Q8_0",
				Object: "list",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.RerankResponse{}, "Data", "Created", "Usage"),
				)

				if diff != "" {
					return diff
				}

				gotResp, ok := got.(*model.RerankResponse)
				if !ok {
					return fmt.Sprintf("response wrong type: %T", got)
				}

				if len(gotResp.Data) != 2 {
					return fmt.Sprintf("expected length of 2 (top_n), got %d", len(gotResp.Data))
				}

				// Check scores are in valid range [0, 1].
				for i, result := range gotResp.Data {
					if result.RelevanceScore < 0 || result.RelevanceScore > 1 {
						return fmt.Sprintf("score out of range [0,1]: index %d, score %.4f", i, result.RelevanceScore)
					}
				}

				// Check results are sorted by relevance (descending).
				if len(gotResp.Data) > 1 && gotResp.Data[0].RelevanceScore < gotResp.Data[1].RelevanceScore {
					return "results not sorted by relevance"
				}

				// Check return_documents works.
				for i, result := range gotResp.Data {
					if result.Document == "" {
						return fmt.Sprintf("expected document at index %d", i)
					}
				}

				if gotResp.Usage.PromptTokens == 0 {
					return "expected prompt tokens to be non-zero"
				}

				return ""
			},
		},
	}

	return table
}

func rerank401(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/rerank",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: model.D{
				"model": "bge-reranker-v2-m3-Q8_0",
				"query": "What is the capital of France?",
				"documents": []string{
					"Paris is the capital of France.",
				},
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"rerank\" not authorized",
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
			URL:        "/v1/rerank",
			Token:      tokens["admin"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: model.D{
				"model": "bge-reranker-v2-m3-Q8_0",
				"query": "What is the capital of France?",
				"documents": []string{
					"Paris is the capital of France.",
				},
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"rerank\" not authorized",
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
