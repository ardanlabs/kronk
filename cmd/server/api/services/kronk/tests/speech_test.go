package chatapi_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
)

func Test_API_SpeechContracts(t *testing.T) {
	test := apitest.NewWithModelConfig(t, "Test_API_SpeechContracts", "testdata/model_config.yaml")
	token := speechToken(t, test, false, "speech")
	var table []apitest.Table
	for _, tc := range []struct {
		name, body, message string
		status              int
	}{
		{name: "empty", body: ``, status: 400},
		{name: "malformed", body: `{"model":`, status: 400},
		{name: "trailing JSON", body: `{"model":"missing","input":"Hello","voice":"default"} {}`, status: 400},
		{name: "null", body: `null`, status: 400},
		{name: "missing model", body: `{"input":"Hello","voice":"default"}`, status: 400, message: "model"},
		{name: "null model", body: `{"model":null,"input":"Hello","voice":"default"}`, status: 400, message: "model"},
		{name: "missing input", body: `{"model":"missing","voice":"default"}`, status: 400, message: "input"},
		{name: "blank input", body: `{"model":"missing","input":" \n","voice":"default"}`, status: 400, message: "input"},
		{name: "missing voice", body: `{"model":"missing","input":"Hello"}`, status: 400, message: "voice"},
		{name: "wrong type", body: `{"model":"missing","input":3,"voice":"default"}`, status: 400},
		{name: "format type", body: `{"model":"missing","input":"Hello","voice":"default","response_format":3}`, status: 400},
		{name: "unsupported format", body: `{"model":"missing","input":"Hello","voice":"default","response_format":"mp3"}`, status: 400, message: "wav, pcm"},
		{name: "SSE", body: `{"model":"missing","input":"Hello","voice":"default","stream_format":"sse"}`, status: 400, message: "buffered"},
		{name: "invalid UTF-8", body: "{\"model\":\"missing\",\"input\":\"\xff\",\"voice\":\"default\"}", status: 400, message: "UTF-8"},
		{name: "body limit", body: strings.Repeat(" ", 1<<20) + `{}`, status: 400, message: "request body too large"},
		{name: "invalid model ID", body: `{"model":"missing","input":"Hello","voice":"default"}`, status: 400},
		{name: "model not found", body: `{"model":"test/missing","input":"Hello","voice":"default"}`, status: 404},
		{name: "null optional fields", body: `{"model":"test/missing","input":"Hello","voice":"default","response_format":null,"stream_format":null}`, status: 404},
		{name: "ignored controls", body: `{"model":"test/missing","input":"Hello","voice":"default","speed":{"any":true},"instructions":[1],"language":"es","max_frames":1}`, status: 404},
		{name: "escaped SDK input limit", body: `{"model":"test/missing","input":"` + strings.Repeat(`\u0061`, 65536) + `","voice":"default"}`, status: 404},
	} {
		code := errs.InvalidArgument
		if tc.status == http.StatusNotFound {
			code = errs.NotFound
		}
		table = append(table, apitest.Table{
			Name: tc.name, URL: "/v1/audio/speech", Method: http.MethodPost,
			Token: token, Headers: map[string]string{"Content-Type": "application/json"},
			RawBody: []byte(tc.body), StatusCode: tc.status,
			GotResp: &errs.Error{}, ExpResp: &errs.Error{Code: code, Message: tc.message},
			ResponseCmpFunc: jsonContentType,
			CmpFunc: func(got, exp any) string {
				g, e := got.(*errs.Error), exp.(*errs.Error)
				if !g.Code.Equal(e.Code) || g.Message == "" || !strings.Contains(g.Message, e.Message) {
					return fmt.Sprintf("got %+v, want %s containing %q", g, e.Code, e.Message)
				}
				return ""
			},
		})
	}
	test.Run(t, table, "speech-request")
	table = nil
	for _, tc := range []struct {
		name, token string
		status      int
	}{
		{name: "missing token", status: 401},
		{name: "invalid JWT", token: "not-a-jwt", status: 401},
		{name: "no grants", token: speechToken(t, test, false, ""), status: 403},
		{name: "transcriptions grant", token: speechToken(t, test, false, "transcriptions"), status: 403},
		{name: "speech grant", token: token, status: 400},
		{name: "admin", token: speechToken(t, test, true, ""), status: 400},
	} {
		code := errs.InvalidArgument
		switch tc.status {
		case http.StatusUnauthorized:
			code = errs.Unauthenticated
		case http.StatusForbidden:
			code = errs.PermissionDenied
		}
		table = append(table, apitest.Table{
			Name: tc.name, URL: "/v1/audio/speech", Method: http.MethodPost,
			Token: tc.token, Input: struct{}{}, StatusCode: tc.status,
			GotResp: &errs.Error{}, ExpResp: &errs.Error{Code: code},
			ResponseCmpFunc: jsonContentType,
			CmpFunc: func(got, exp any) string {
				g, e := got.(*errs.Error), exp.(*errs.Error)
				if !g.Code.Equal(e.Code) {
					return fmt.Sprintf("got error code %s, want %s", g.Code, e.Code)
				}
				// Authorized requests reach validation without loading a model.
				if e.Code.Equal(errs.InvalidArgument) && g.Message != "missing model field" {
					return fmt.Sprintf("authorized request did not reach validation: %+v", g)
				}
				return ""
			},
		})
	}
	test.Run(t, table, "speech-authorization")
}

func speechToken(t *testing.T, test *apitest.Test, admin bool, endpoint string) string {
	t.Helper()
	var endpoints map[string]auth.RateLimit
	if endpoint != "" {
		endpoints = map[string]auth.RateLimit{endpoint: {Window: auth.RateUnlimited}}
	}
	token, err := test.Sec.GenerateToken(admin, endpoints, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return token
}
