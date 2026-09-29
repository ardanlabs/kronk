package chatapi_test

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

const decisionModelID = "chaoliangUNSW/Jev-Style-0.8B-Decision-v3-Q8_0"

func decisionRequest() model.D {
	return model.D{
		"model": decisionModelID,
		"state": model.D{
			"customer_message": "I was charged twice and need this fixed today.",
			"account_tier":     "business",
		},
		"questions": model.D{
			"route": model.D{
				"type":         "choice",
				"instructions": "Which team should handle this request?",
				"criteria": model.D{
					"billing":           "Payments, invoices, refunds, and duplicate charges",
					"technical_support": "Product bugs and technical problems",
					"sales":             "Plans, pricing, and new purchases",
				},
			},
			"urgency": model.D{
				"type":         "score",
				"instructions": "How urgent is this request?",
				"criteria":     []string{"not urgent", "normal", "urgent", "critical"},
			},
			"requires_human": model.D{
				"type":         "noul",
				"instructions": "Should a human review this request?",
			},
		},
	}
}

func decision200(tokens map[string]string) []apitest.Table {
	var table []apitest.Table

	for _, path := range []string{"/v1/systemone", "/v1/decide"} {
		table = append(table, apitest.Table{
			Name:       path,
			URL:        path,
			Token:      tokens["decision"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input:      decisionRequest(),
			GotResp:    &model.DecisionResponse{},
			ExpResp: &model.DecisionResponse{
				Model: decisionModelID,
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(model.DecisionResponse{}, "Answers", "Usage"),
				)
				if diff != "" {
					return diff
				}

				gotResp, ok := got.(*model.DecisionResponse)
				if !ok {
					return fmt.Sprintf("response wrong type: %T", got)
				}

				for id, questionType := range map[string]model.DecisionQuestionType{
					"route":          model.DecisionQuestionTypeChoice,
					"urgency":        model.DecisionQuestionTypeScore,
					"requires_human": model.DecisionQuestionTypeNoul,
				} {
					answer, exists := gotResp.Answers[id]
					if !exists {
						return fmt.Sprintf("answer %q is missing", id)
					}
					if answer.Type != questionType {
						return fmt.Sprintf("answer %q type: got %q, want %q", id, answer.Type, questionType)
					}
				}

				if gotResp.Usage.InputTokens == 0 {
					return "expected input tokens to be non-zero"
				}

				return ""
			},
		})
	}

	return table
}

func decisionNegative(tokens map[string]string) []apitest.Table {
	type testCase struct {
		name       string
		input      any
		rawBody    []byte
		statusCode int
		code       errs.ErrCode
		message    string
	}

	tests := []testCase{
		{
			name:       "malformed-json",
			rawBody:    []byte(`{"model":`),
			statusCode: http.StatusBadRequest,
			code:       errs.InvalidArgument,
			message:    "decode decision request",
		},
		{
			name: "unknown-question-type",
			input: model.D{
				"model": decisionModelID,
				"state": model.D{},
				"questions": model.D{
					"route": model.D{
						"type":         "unsupported",
						"instructions": "Route this request",
					},
				},
			},
			statusCode: http.StatusBadRequest,
			code:       errs.InvalidArgument,
			message:    `unknown type "unsupported"`,
		},
		{
			name: "invalid-model-id",
			input: model.D{
				"model":     "not-a-model-id",
				"state":     model.D{},
				"questions": decisionRequest()["questions"],
			},
			statusCode: http.StatusBadRequest,
			code:       errs.InvalidArgument,
			message:    "invalid model id",
		},
		{
			name: "model-not-found",
			input: model.D{
				"model":     "missing/not-installed-decision-model",
				"state":     model.D{},
				"questions": decisionRequest()["questions"],
			},
			statusCode: http.StatusNotFound,
			code:       errs.NotFound,
			message:    "model not found",
		},
		{
			name: "unsupported-model",
			input: model.D{
				"model":     qwen3ModelID,
				"state":     model.D{},
				"questions": decisionRequest()["questions"],
			},
			statusCode: http.StatusBadRequest,
			code:       errs.InvalidArgument,
			message:    "model doesn't support decisions",
		},
		{
			name: "token-budget-overflow",
			input: model.D{
				"model": decisionModelID,
				"state": model.D{
					"oversized": strings.Repeat("state ", 30_000),
				},
				"questions": decisionRequest()["questions"],
			},
			statusCode: http.StatusBadRequest,
			code:       errs.InvalidArgument,
			message:    "decision input exceeds token budget",
		},
	}

	var table []apitest.Table
	for _, path := range []string{"/v1/systemone", "/v1/decide"} {
		for _, tt := range tests {
			table = append(table, apitest.Table{
				Name:       path + "-" + tt.name,
				URL:        path,
				Token:      tokens["decision"],
				Method:     http.MethodPost,
				StatusCode: tt.statusCode,
				Input:      tt.input,
				RawBody:    tt.rawBody,
				GotResp:    &errs.Error{},
				CmpFunc: func(got any, _ any) string {
					gotErr := got.(*errs.Error)
					if gotErr.Code != tt.code {
						return fmt.Sprintf("error code: got %s, want %s", gotErr.Code, tt.code)
					}
					if !strings.Contains(gotErr.Message, tt.message) {
						return fmt.Sprintf("error message %q does not contain %q", gotErr.Message, tt.message)
					}

					return ""
				},
			})
		}
	}

	return table
}

func decision403(tokens map[string]string) []apitest.Table {
	var table []apitest.Table

	for _, path := range []string{"/v1/systemone", "/v1/decide"} {
		table = append(table, apitest.Table{
			Name:       path,
			URL:        path,
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusForbidden,
			Input:      decisionRequest(),
			GotResp:    &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.PermissionDenied,
				Message: "rpc error: code = PermissionDenied desc = permission denied",
			},
			CmpFunc: func(got any, exp any) string {
				return cmp.Diff(got, exp,
					cmpopts.IgnoreFields(errs.Error{}, "FuncName", "FileName"),
				)
			},
		})
	}

	return table
}
