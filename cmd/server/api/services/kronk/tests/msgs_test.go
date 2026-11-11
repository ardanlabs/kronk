package chatapi_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/msgsapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// =============================================================================
// Tests grouped by model to minimize model loading/unloading in CI.
// =============================================================================

// msgsNonStreamQwen3 returns messages tests for the Qwen3-1.7B model (text).
func msgsNonStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/messages",
			Token:      tokens["messages"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: msgsapp.MessagesRequest{
				Model:     qwen3ModelID,
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
				Model: qwen3ModelID,
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

// msgsStreamQwen3 returns streaming messages tests for the Qwen3-1.7B model.
func msgsStreamQwen3(_ *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/messages",
			Token:      tokens["messages"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: msgsapp.MessagesRequest{
				Model:     qwen3ModelID,
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
			SSECmpFunc: func(events []apitest.SSEEvent) string {
				got, diff := validateMsgsStream(events, qwen3ModelID, false)
				if diff == "" && strings.TrimSpace(got.Text) == "" {
					return "expected non-empty reconstructed text"
				}
				if diff == "" && (got.Usage.InputTokens <= 0 || got.Usage.OutputTokens <= 0) {
					return "expected positive final input and output usage"
				}
				return diff
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

// msgsToolRoundTrip exercises tool selection, typed arguments, result history,
// and final text through the production routes with both response transports.
func msgsToolRoundTrip(t *testing.T, api *apitest.Test, token string) {
	t.Helper()
	for _, stream := range []bool{false, true} {
		name := "nonstream"
		if stream {
			name = "stream"
		}
		t.Run(name, func(t *testing.T) {
			request := func(input msgsapp.MessagesRequest) ([]msgsapp.ResponseContentBlock, string) {
				t.Helper()
				data, err := json.Marshal(input)
				if err != nil {
					t.Fatal(err)
				}
				r := httptest.NewRequest(http.MethodPost, "/v1/messages", bytes.NewReader(data))
				r.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				api.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					t.Fatalf("messages: got %d %s", w.Code, w.Body)
				}
				if id := w.Header().Get("request-id"); id == "" || id != w.Header().Get("anthropic-request-id") {
					t.Fatalf("request IDs: got %v", w.Header())
				}
				if input.Stream {
					var events []apitest.SSEEvent
					for record := range strings.SplitSeq(strings.TrimSpace(w.Body.String()), "\n\n") {
						event, data, ok := strings.Cut(record, "\ndata: ")
						if !ok {
							t.Fatalf("malformed SSE: %q", record)
						}
						events = append(events, apitest.SSEEvent{Event: strings.TrimPrefix(event, "event: "), Data: json.RawMessage(data)})
					}
					got, diff := validateMsgsStream(events, qwen3ModelID, false)
					if diff != "" {
						t.Fatal(diff)
					}
					if got.Usage.InputTokens <= 0 || got.Usage.OutputTokens <= 0 {
						t.Fatalf("final usage: got %+v", got.Usage)
					}
					content := got.Tools
					if got.Text != "" {
						content = append([]msgsapp.ResponseContentBlock{{Type: "text", Text: got.Text}}, content...)
					}
					return content, got.StopReason
				}
				var got msgsapp.MessagesResponse
				if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if got.Usage.InputTokens <= 0 || got.Usage.OutputTokens <= 0 {
					t.Fatalf("final usage: got %+v", got.Usage)
				}
				return got.Content, got.StopReason
			}
			input := msgsapp.MessagesRequest{
				Model: qwen3ModelID, MaxTokens: 2048, Stream: stream, Temperature: new(0.0),
				System:   msgsapp.SystemContent{Text: "Follow the user's instructions. When given a tool result, repeat its text exactly."},
				Messages: []msgsapp.Message{{Role: "user", Content: msgsapp.Content{Text: "Call lookup with code 42."}}},
				Tools: []msgsapp.Tool{{Name: "lookup", Description: "Look up a code", InputSchema: model.D{
					"type": "object", "properties": model.D{"code": model.D{"type": "integer"}}, "required": []string{"code"}, "additionalProperties": false,
				}}},
				ToolChoice: &msgsapp.ToolChoice{Type: "tool", Name: "lookup"},
			}
			content, stop := request(input)
			var assistant []msgsapp.ContentBlock
			var callID string
			for _, block := range content {
				assistant = append(assistant, msgsapp.ContentBlock{Type: block.Type, Text: block.Text, ID: block.ID, Name: block.Name, Input: block.Input})
				if block.Type == "tool_use" {
					args, ok := block.Input.(map[string]any)
					if !ok || args["code"] != float64(42) || block.Name != "lookup" || block.ID == "" || callID != "" {
						t.Fatalf("tool call: got %+v, want one lookup with integer code 42", block)
					}
					callID = block.ID
				}
			}
			if stop != "tool_use" || callID == "" {
				t.Fatalf("tool completion: got %s %+v", stop, content)
			}
			input.Messages = append(input.Messages,
				msgsapp.Message{Role: "assistant", Content: msgsapp.Content{Blocks: assistant}},
				msgsapp.Message{Role: "user", Content: msgsapp.Content{Blocks: []msgsapp.ContentBlock{
					{Type: "tool_result", ToolUseID: callID, Content: msgsapp.Content{Blocks: []msgsapp.ContentBlock{{Type: "text", Text: "ZEBRA-417"}}}},
					{Type: "text", Text: "Repeat the tool result text exactly."},
				}}},
			)
			input.ToolChoice = &msgsapp.ToolChoice{Type: "none"}
			content, stop = request(input)
			var text strings.Builder
			for _, block := range content {
				if block.Type != "text" {
					t.Fatalf("unexpected follow-up block: %+v", block)
				}
				text.WriteString(block.Text)
			}
			if stop != "end_turn" || !strings.Contains(text.String(), "ZEBRA-417") {
				t.Fatalf("follow-up: got %s %q, want end_turn with ZEBRA-417", stop, text.String())
			}
		})
	}
}

// msgsImageQwen35 returns messages tests for the Qwen3.5 vision model.
func msgsImageQwen35(t *testing.T, tokens map[string]string) []apitest.Table {
	image, err := readFile(imageFile)
	if err != nil {
		t.Fatalf("read image: %s", err)
	}

	return []apitest.Table{
		{
			Name:       "image-good-token",
			URL:        "/v1/messages",
			Token:      tokens["messages"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: msgsapp.MessagesRequest{
				Model:     visionModelID,
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
										Data:      base64.StdEncoding.EncodeToString(image),
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
				Model: visionModelID,
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

func msgsEndpoint403(tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/messages",
			Token:      tokens["embeddings"],
			Method:     http.MethodPost,
			StatusCode: http.StatusForbidden,
			Input: msgsapp.MessagesRequest{
				Model:     qwen3ModelID,
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
			GotResp: &msgsapp.ErrorResponse{},
			ExpResp: &msgsapp.ErrorResponse{
				Type: "error",
				Error: msgsapp.ErrorDetail{
					Type:    "permission_error",
					Message: "rpc error: code = PermissionDenied desc = permission denied",
				},
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
		switch {
		case len(err) > 6 && err[:6] == "[WARN]":
			t.Log(err)

		default:
			return err
		}
	}

	return ""
}
