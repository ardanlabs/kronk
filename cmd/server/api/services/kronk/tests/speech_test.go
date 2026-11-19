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
		{name: "unsupported format", body: `{"model":"missing","input":"Hello","voice":"default","response_format":"ogg"}`, status: 400, message: "wav, pcm, mp3, opus, aac, flac"},
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

	limited, err := test.Sec.GenerateToken(false, map[string]auth.RateLimit{
		"speech":         {Limit: 1, Window: auth.RateDay},
		"transcriptions": {Limit: 1, Window: auth.RateDay},
	}, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	table = nil
	for _, tc := range []struct {
		name, path, token string
		status            int
		code              errs.ErrCode
	}{
		{"speech cannot transcribe", "/v1/audio/transcriptions", token, 403, errs.PermissionDenied},
		{"speech cannot translate", "/v1/audio/translations", token, 403, errs.PermissionDenied},
		{"first speech reaches validation", "/v1/audio/speech", limited, 400, errs.InvalidArgument},
		{"second speech rate limited", "/v1/audio/speech", limited, 429, errs.TooManyRequests},
		{"transcription has independent quota", "/v1/audio/transcriptions", limited, 400, errs.InvalidArgument},
		{"translation shares transcription quota", "/v1/audio/translations", limited, 429, errs.TooManyRequests},
		{"unlimited speech still allowed", "/v1/audio/speech", token, 400, errs.InvalidArgument},
	} {
		table = append(table, apitest.Table{
			Name: tc.name, URL: tc.path, Method: http.MethodPost, Token: tc.token,
			Input: struct{}{}, StatusCode: tc.status,
			GotResp: &errs.Error{}, ExpResp: tc.code, ResponseCmpFunc: jsonContentType,
			CmpFunc: func(got, exp any) string {
				g := got.(*errs.Error)
				if !g.Code.Equal(exp.(errs.ErrCode)) || g.Message == "" {
					return fmt.Sprintf("got %+v, want error code %s with a message", g, exp)
				}
				return ""
			},
		})
	}
	// Authorized invalid bodies consume quota before validation, without
	// loading models. Keep these requests ordered to check each boundary.
	test.Run(t, table, "speech-grants-and-limits")

	t.Run("ffmpeg missing", func(t *testing.T) {
		t.Setenv("PATH", "")
		var table []apitest.Table
		for _, format := range []string{"mp3", "opus", "aac", "flac", "wav", "pcm"} {
			status, code, message := http.StatusBadRequest, errs.FailedPrecondition, "requires ffmpeg on the server PATH"
			if format == "wav" || format == "pcm" {
				// These formats bypass ffmpeg and reach model lookup.
				status, code, message = http.StatusNotFound, errs.NotFound, ""
			}
			table = append(table, apitest.Table{
				Name: format, URL: "/v1/audio/speech", Method: http.MethodPost, Token: token,
				RawBody:    fmt.Appendf(nil, `{"model":"test/missing","input":"Hello","voice":"default","response_format":%q}`, format),
				StatusCode: status, GotResp: &errs.Error{}, ResponseCmpFunc: jsonContentType,
				CmpFunc: func(got, _ any) string {
					g := got.(*errs.Error)
					if !g.Code.Equal(code) || !strings.Contains(g.Message, message) {
						return fmt.Sprintf("got %+v, want %s containing %q", g, code, message)
					}
					return ""
				},
			})
		}
		test.Run(t, table, "speech-no-ffmpeg")
	})
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
