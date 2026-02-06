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

func tokenize200(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "raw-input",
			URL:        "/v1/tokenize",
			Token:      tokens["tokenize"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"input": "The quick brown fox jumps over the lazy dog",
			},
			GotResp: &model.TokenizeResponse{},
			ExpResp: &model.TokenizeResponse{
				Model:  "Qwen3-8B-Q8_0",
				Object: "tokenize",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.TokenizeResponse{}, "Created", "Tokens"),
				)

				if diff != "" {
					return diff
				}

				gotResp, ok := got.(*model.TokenizeResponse)
				if !ok {
					return fmt.Sprintf("response wrong type: %T", got)
				}

				if gotResp.Tokens == 0 {
					return "expected non-zero token count"
				}

				return ""
			},
		},
		{
			Name:       "with-template",
			URL:        "/v1/tokenize",
			Token:      tokens["tokenize"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":          "Qwen3-8B-Q8_0",
				"input":          "The quick brown fox jumps over the lazy dog",
				"apply_template": true,
			},
			GotResp: &model.TokenizeResponse{},
			ExpResp: &model.TokenizeResponse{
				Model:  "Qwen3-8B-Q8_0",
				Object: "tokenize",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.TokenizeResponse{}, "Created", "Tokens"),
				)

				if diff != "" {
					return diff
				}

				gotResp, ok := got.(*model.TokenizeResponse)
				if !ok {
					return fmt.Sprintf("response wrong type: %T", got)
				}

				if gotResp.Tokens == 0 {
					return "expected non-zero token count"
				}

				return ""
			},
		},
	}

	return table
}

func tokenize401(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/tokenize",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"input": "hello",
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"tokenize\" not authorized",
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
			URL:        "/v1/tokenize",
			Token:      tokens["admin"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: model.D{
				"model": "Qwen3-8B-Q8_0",
				"input": "hello",
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"tokenize\" not authorized",
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
