package chatapi_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/msgsapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// =============================================================================
// Tests grouped by model to minimize model loading/unloading in CI.
// =============================================================================

// msgsNonStreamQwen3 returns messages tests for Qwen3-8B-Q8_0 model (text).
func msgsNonStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			SkipInGH:   true,
			URL:        "/v1/messages",
			Token:      tokens["messages"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: msgsapp.MessagesRequest{
				Model:     "Qwen3-8B-Q8_0",
				MaxTokens: 2048,
				Messages: []msgsapp.Message{
					{
						Role: "user",
						Content: msgsapp.Content{
							Text: "Echo back the word: Gorilla",
						},
					},
				},
			},
			GotResp: &msgsapp.MessagesResponse{},
			ExpResp: &msgsapp.MessagesResponse{
				Type:  "message",
				Role:  "assistant",
				Model: "Qwen3-8B-Q8_0",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(msgsapp.MessagesResponse{}, "ID", "Content", "StopReason", "StopSequence", "Usage"),
				)

				if diff != "" {
					return diff
				}

				return validateMsgsResponse(got).
					hasValidID().
					hasType("message").
					hasRole("assistant").
					hasContent().
					warnContainsInContent("gorilla").
					result(t)
			},
		},
	}
}

// msgsStreamQwen3 returns streaming messages tests for Qwen3-8B-Q8_0 model.
func msgsStreamQwen3(_ *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			SkipInGH:   true,
			URL:        "/v1/messages",
			Token:      tokens["messages"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: msgsapp.MessagesRequest{
				Model:     "Qwen3-8B-Q8_0",
				MaxTokens: 2048,
				Stream:    true,
				Messages: []msgsapp.Message{
					{
						Role: "user",
						Content: msgsapp.Content{
							Text: "Echo back the word: Gorilla",
						},
					},
				},
			},
			GotResp: &msgsapp.MessageStopEvent{},
			ExpResp: &msgsapp.MessageStopEvent{
				Type: "message_stop",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp)
				if diff != "" {
					return diff
				}
				return ""
			},
		},
	}
}

// msgsImageQwen25VL returns messages tests for Qwen2.5-VL-3B-Instruct-Q8_0 model (vision).
func msgsImageQwen25VL(t *testing.T, tokens map[string]string) []apitest.Table {
	image, err := readFile(imageFile)
	if err != nil {
		t.Fatalf("read image: %s", err)
	}

	return []apitest.Table{
		{
			Name:       "image-good-token",
			SkipInGH:   true,
			URL:        "/v1/messages",
			Token:      tokens["messages"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: msgsapp.MessagesRequest{
				Model:     "Qwen2.5-VL-3B-Instruct-Q8_0",
				MaxTokens: 2048,
				Messages: []msgsapp.Message{
					{
						Role: "user",
						Content: msgsapp.Content{
							Blocks: []msgsapp.ContentBlock{
								{
									Type: "image",
									Source: &msgsapp.ImageSource{
										Type:      "base64",
										MediaType: "image/jpeg",
										Data:      string(image),
									},
								},
								{
									Type: "text",
									Text: "what's in the picture",
								},
							},
						},
					},
				},
			},
			GotResp: &msgsapp.MessagesResponse{},
			ExpResp: &msgsapp.MessagesResponse{
				Type:  "message",
				Role:  "assistant",
				Model: "Qwen2.5-VL-3B-Instruct-Q8_0",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(msgsapp.MessagesResponse{}, "ID", "Content", "StopReason", "StopSequence", "Usage"),
				)

				if diff != "" {
					return diff
				}

				return validateMsgsResponse(got).
					hasValidID().
					hasType("message").
					hasRole("assistant").
					hasContent().
					warnContainsInContent("giraffes").
					result(t)
			},
		},
	}
}

// =============================================================================

func msgsEndpoint401(tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "bad-token",
			SkipInGH:   true,
			URL:        "/v1/messages",
			Token:      tokens["embeddings"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: msgsapp.MessagesRequest{
				Model:     "Qwen3-8B-Q8_0",
				MaxTokens: 2048,
				Messages: []msgsapp.Message{
					{
						Role: "user",
						Content: msgsapp.Content{
							Text: "Echo back the word: Gorilla",
						},
					},
				},
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"messages\" not authorized",
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
			SkipInGH:   true,
			URL:        "/v1/messages",
			Token:      tokens["admin"],
			Method:     http.MethodPost,
			StatusCode: http.StatusUnauthorized,
			Input: msgsapp.MessagesRequest{
				Model:     "Qwen3-8B-Q8_0",
				MaxTokens: 2048,
				Messages: []msgsapp.Message{
					{
						Role: "user",
						Content: msgsapp.Content{
							Text: "Echo back the word: Gorilla",
						},
					},
				},
			},
			GotResp: &errs.Error{},
			ExpResp: &errs.Error{
				Code:    errs.Unauthenticated,
				Message: "rpc error: code = Unauthenticated desc = not authorized: attempted action is not allowed: endpoint \"messages\" not authorized",
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
}

// =============================================================================
// Validation helpers

type msgsResponseValidator struct {
	resp   *msgsapp.MessagesResponse
	errors []string
}

func validateMsgsResponse(got any) msgsResponseValidator {
	resp, ok := got.(*msgsapp.MessagesResponse)
	if !ok {
		return msgsResponseValidator{errors: []string{"expected *msgsapp.MessagesResponse"}}
	}
	return msgsResponseValidator{resp: resp}
}

func (v msgsResponseValidator) hasValidID() msgsResponseValidator {
	if v.resp == nil {
		return v
	}
	if v.resp.ID == "" {
		v.errors = append(v.errors, "expected non-empty ID")
	}
	return v
}

func (v msgsResponseValidator) hasType(expected string) msgsResponseValidator {
	if v.resp == nil {
		return v
	}
	if v.resp.Type != expected {
		v.errors = append(v.errors, "expected type "+expected+", got "+v.resp.Type)
	}
	return v
}

func (v msgsResponseValidator) hasRole(expected string) msgsResponseValidator {
	if v.resp == nil {
		return v
	}
	if v.resp.Role != expected {
		v.errors = append(v.errors, "expected role "+expected+", got "+v.resp.Role)
	}
	return v
}

func (v msgsResponseValidator) hasContent() msgsResponseValidator {
	if v.resp == nil {
		return v
	}
	if len(v.resp.Content) == 0 {
		v.errors = append(v.errors, "expected non-empty content")
	}
	return v
}

func (v msgsResponseValidator) warnContainsInContent(substr string) msgsResponseValidator {
	if v.resp == nil {
		return v
	}
	for _, block := range v.resp.Content {
		if block.Type == "text" && containsIgnoreCase(block.Text, substr) {
			return v
		}
	}
	v.errors = append(v.errors, "[WARN] content does not contain '"+substr+"', got: "+v.extractContent())
	return v
}

func (v msgsResponseValidator) extractContent() string {
	var texts []string
	for _, block := range v.resp.Content {
		if block.Type == "text" && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	return strings.Join(texts, " | ")
}

func (v msgsResponseValidator) result(t *testing.T) string {
	for _, err := range v.errors {
		if len(err) > 6 && err[:6] == "[WARN]" {
			t.Log(err)
		} else {
			return err
		}
	}
	return ""
}
