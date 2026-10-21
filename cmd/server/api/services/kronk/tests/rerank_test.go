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

func rerank200(tokens map[string]string) []apitest.Table {
	documents := []string{
		"Paris is the capital and largest city of France.",
		"Berlin is the capital of Germany.",
		"The Eiffel Tower is located in Paris.",
	}
	table := []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/rerank",
			Token:      tokens["rerank"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":            "gpustack/bge-reranker-v2-m3-Q8_0",
				"query":            "What is the capital of France?",
				"documents":        documents,
				"top_n":            2,
				"return_documents": true,
			},
			GotResp: &model.RerankResponse{},
			ExpResp: &model.RerankResponse{
				Model:  "gpustack/bge-reranker-v2-m3-Q8_0",
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

				if err := testlib.CheckRerank(*gotResp, "gpustack/bge-reranker-v2-m3-Q8_0", documents, 2, true); err != nil {
					return err.Error()
				}
				if gotResp.Data[0].Index != 0 {
					return "expected Paris/France at source index 0 to rank first"
				}

				return ""
			},
		},
	}

	return table
}

func rerank403(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/rerank",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusForbidden,
			Input: model.D{
				"model": "gpustack/bge-reranker-v2-m3-Q8_0",
				"query": "What is the capital of France?",
				"documents": []string{
					"Paris is the capital of France.",
				},
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
