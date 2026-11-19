package chatapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/msgsapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mux"
)

func Test_API_MessagesValidation(t *testing.T) {
	// No pool: invalid requests must be rejected before model acquisition.
	api := newTransportAPI(t, mux.Config{})
	for _, stream := range []string{"false", "true"} {
		var table []apitest.Table
		for i, body := range []string{
			`{"model":"test","max_tokens":1,"stream":` + stream + `}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"stop_sequences":["END"],"stream":` + stream + `}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"stream":` + stream + `}{"second":true}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"stream":` + stream + `}42`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"stream":` + stream + `} trailing-garbage`,
			`{"model":"test","max_tokens":-1,"messages":[{"role":"user","content":"hello"}],"stream":` + stream + `}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"system","content":"hello"}],"stream":` + stream + `}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":{}}],"stream":` + stream + `}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"system":42,"stream":` + stream + `}`,
			`{"max_tokens":1,"stream":` + stream + `} ` + "\n\t",
		} {
			table = append(table, apitest.Table{
				Name: fmt.Sprintf("invalid-body-%d", i), URL: "/v1/messages", Method: http.MethodPost,
				RawBody: []byte(body), StatusCode: http.StatusBadRequest,
				GotResp: &msgsapp.ErrorResponse{},
				ExpResp: &msgsapp.ErrorResponse{Type: "error", Error: msgsapp.ErrorDetail{Type: "invalid_request_error"}},
				CmpFunc: func(got, exp any) string {
					g, e := got.(*msgsapp.ErrorResponse), exp.(*msgsapp.ErrorResponse)
					if g.Type != e.Type || g.Error.Type != e.Error.Type || g.Error.Message == "" {
						return fmt.Sprintf("invalid error response: %+v", g)
					}
					if strings.Contains(body, "stop_sequences") && g.Error.Message != "stop_sequences is not supported" {
						return fmt.Sprintf("stop sequences: got %q, want unsupported error", g.Error.Message)
					}
					return ""
				},
				ResponseCmpFunc: func(header http.Header, body []byte) string {
					if diff := jsonContentType(header, body); diff != "" {
						return diff
					}
					if id := header.Get("request-id"); id == "" || id != header.Get("anthropic-request-id") {
						return fmt.Sprintf("request ID headers: got %v", header)
					}
					return ""
				},
			})
		}
		api.Run(t, table, "messages-validation-stream-"+stream)
	}
}

func Test_API_ResponsesValidation(t *testing.T) {
	api := newTransportAPI(t, mux.Config{})
	var table []apitest.Table
	for _, stream := range []string{"false", "true"} {
		for i, suffix := range []string{`{"second":true}`, `42`, ` trailing-garbage`} {
			body := `{"model":"unavailable","input":"hello","stream":` + stream + `}` + suffix
			table = append(table, apitest.Table{
				Name: fmt.Sprintf("stream-%s-suffix-%d", stream, i),
				URL:  "/v1/responses", Method: http.MethodPost, RawBody: []byte(body),
				StatusCode: http.StatusBadRequest, GotResp: &errs.Error{},
				ResponseCmpFunc: jsonContentType,
				CmpFunc: func(got, _ any) string {
					g := got.(*errs.Error)
					if !g.Code.Equal(errs.InvalidArgument) || g.Message == "" {
						return fmt.Sprintf("got %+v, want invalid request error", g)
					}
					return ""
				},
			})
		}
	}
	// Whitespace after a valid object reaches model-field validation.
	table = append(table, apitest.Table{
		Name: "trailing-whitespace", URL: "/v1/responses", Method: http.MethodPost,
		RawBody: []byte("{} \n\t"), StatusCode: http.StatusBadRequest,
		GotResp: &errs.Error{}, ResponseCmpFunc: jsonContentType,
		CmpFunc: func(got, _ any) string {
			g := got.(*errs.Error)
			if !g.Code.Equal(errs.InvalidArgument) || g.Message != "missing model field" {
				return fmt.Sprintf("got %+v, want missing model field", g)
			}
			return ""
		},
	})
	api.Run(t, table, "responses-validation")
}

func Test_API_DecisionRoutes(t *testing.T) {
	api := newTransportAPI(t, mux.Config{})
	var table []apitest.Table
	for _, path := range []string{"/v1/systemone", "/v1/decide"} {
		table = append(table, apitest.Table{
			Name: path, URL: path, Method: http.MethodPost, Input: struct{}{},
			StatusCode: http.StatusBadRequest, GotResp: &errs.Error{},
			ResponseCmpFunc: jsonContentType,
			CmpFunc: func(got, _ any) string {
				if g := got.(*errs.Error); !g.Code.Equal(errs.InvalidArgument) || g.Message == "" {
					return fmt.Sprintf("got %+v, want invalid request error", g)
				}
				return ""
			},
		})
	}
	api.Run(t, table, "decision-routes")
}
