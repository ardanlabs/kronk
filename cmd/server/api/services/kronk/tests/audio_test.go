package chatapi_test

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// audioTranscriptionsResponse mirrors the "json" response_format that
// the /v1/audio/transcriptions endpoint returns. The endpoint always
// emits a flat object with a "text" field for the json variant.
type audioTranscriptionsResponse struct {
	Text string `json:"text"`
}

// buildAudioForm produces a multipart/form-data body suitable for the
// /v1/audio/transcriptions endpoint. Returns the body bytes and the
// Content-Type header (which includes the random boundary).
func buildAudioForm(t *testing.T, audioPath, modelID, language, respFmt string) ([]byte, string) {
	t.Helper()

	data, err := readFile(audioPath)
	if err != nil {
		t.Fatalf("read audio file: %v", err)
	}

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	if err := w.WriteField("model", modelID); err != nil {
		t.Fatalf("write model field: %v", err)
	}
	if language != "" {
		if err := w.WriteField("language", language); err != nil {
			t.Fatalf("write language field: %v", err)
		}
	}
	if respFmt != "" {
		if err := w.WriteField("response_format", respFmt); err != nil {
			t.Fatalf("write response_format field: %v", err)
		}
	}

	part, err := w.CreateFormFile("file", "jfk.wav")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write form file: %v", err)
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	return buf.Bytes(), w.FormDataContentType()
}

func audioTranscriptions200(t *testing.T, tokens map[string]string) []apitest.Table {
	body, contentType := buildAudioForm(t, audioFile, "tiny.en", "en", "json")

	table := []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/audio/transcriptions",
			Token:      tokens["transcriptions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Headers: map[string]string{
				"Content-Type": contentType,
			},
			RawBody: body,
			GotResp: &audioTranscriptionsResponse{},
			ExpResp: &audioTranscriptionsResponse{},
			CmpFunc: func(got any, exp any) string {
				gotResp, ok := got.(*audioTranscriptionsResponse)
				if !ok {
					return fmt.Sprintf("response wrong type: %T", got)
				}

				if strings.TrimSpace(gotResp.Text) == "" {
					return "expected non-empty text in response"
				}

				// The JFK "ask not what your country" clip is famous
				// and whisper output is stable enough for a
				// substring assertion.
				if !strings.Contains(strings.ToLower(gotResp.Text), "ask not") {
					return fmt.Sprintf("expected transcript to contain \"ask not\", got: %q", gotResp.Text)
				}

				return ""
			},
		},
	}

	return table
}

func audioTranscriptions401(t *testing.T, tokens map[string]string) []apitest.Table {
	body, contentType := buildAudioForm(t, audioFile, "tiny.en", "en", "json")

	bodyAdmin, contentTypeAdmin := buildAudioForm(t, audioFile, "tiny.en", "en", "json")

	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/audio/transcriptions",
			Token:      tokens["chat-completions"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Headers: map[string]string{
				"Content-Type": contentType,
			},
			RawBody: body,
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"transcriptions\" not authorized",
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
			URL:        "/v1/audio/transcriptions",
			Token:      tokens["admin"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Headers: map[string]string{
				"Content-Type": contentTypeAdmin,
			},
			RawBody: bodyAdmin,
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"transcriptions\" not authorized",
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
