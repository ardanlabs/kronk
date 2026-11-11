package chatapi_test

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
)

// =============================================================================
// Tests grouped by model to minimize model loading/unloading in CI.
// =============================================================================

// respNonStreamQwen3 returns response tests for the Qwen3-1.7B model (text).
func respNonStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name: "native-schema-exact-number", URL: "/v1/responses", Token: tokens["responses"],
			Method: http.MethodPost, StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID, "input": "Return the required integer.",
				"instructions": "Follow the schema.", "metadata": model.D{"tag": "precision"},
				"temperature": 0.25, "top_p": 0.75, "max_output_tokens": 64, "max_tokens": 1,
				"text": model.D{"format": model.D{"type": "json_schema", "name": "exact", "strict": false,
					"schema": model.D{"type": "integer", "enum": []any{json.Number("9007199254740993")}}}},
			},
			GotResp: &kronk.ResponseResponse{},
			CmpFunc: func(got any, _ any) string {
				r := got.(*kronk.ResponseResponse)
				if r.Status != "completed" || r.MaxOutputTokens == nil || *r.MaxOutputTokens != 64 || r.Temperature != 0.25 || r.TopP != 0.75 {
					return "incorrect native-schema status or numeric metadata"
				}
				if r.Instructions == nil || *r.Instructions != "Follow the schema." || r.Metadata["tag"] != "precision" {
					return "lost instructions or request metadata"
				}
				f := r.Text.Format
				if f.Type != "json_schema" || f.Name != "exact" || f.Schema == nil || f.Strict == nil || *f.Strict {
					return "lost native-schema format metadata"
				}
				var text string
				for _, item := range r.Output {
					if item.Type == "message" {
						for _, part := range item.Content {
							text += part.Text
						}
					}
				}
				if strings.TrimSpace(text) != "9007199254740993" {
					return fmt.Sprintf("generated number: got %q, want exact 9007199254740993", text)
				}
				return ""
			},
		},
		{
			Name:       "good-token",
			URL:        "/v1/responses",
			Token:      tokens["responses"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"input": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &kronk.ResponseResponse{},
			ExpResp: &kronk.ResponseResponse{
				Object:          "response",
				Status:          "completed",
				MaxOutputTokens: new(2048),
				Model:           qwen3ModelID,
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(kronk.ResponseResponse{}, "ID", "CreatedAt", "CompletedAt", "Usage", "Output", "Temperature", "TopP", "ToolChoice", "Truncation", "Tools", "Metadata", "Text", "Reasoning", "ParallelToolCall", "Store"),
				)

				if diff != "" {
					return diff
				}

				return validateRespResponse(got).
					hasValidID().
					hasCreatedAt().
					hasStatus("completed").
					hasOutput().
					hasOutputText().
					warnContainsInContent("gorilla").
					result(t)
			},
		},
	}
	// Mutually exclusive literal outputs prove the winning control reaches
	// inference, rather than merely being reported in format metadata.
	for _, winner := range []string{"native", "legacy", "grammar", "schema"} {
		input := model.D{
			"model": qwen3ModelID, "input": "Return the requested string.",
			"max_output_tokens": 64, "temperature": 0,
			"text": model.D{"format": model.D{"type": "json_schema", "name": "native", "schema": model.D{"type": "string", "enum": []string{"native"}}}},
		}
		if winner != "native" {
			input["response_format"] = model.D{"type": "json_schema", "json_schema": model.D{"name": "legacy", "schema": model.D{"type": "string", "enum": []string{"legacy"}}}}
		}
		if winner == "grammar" || winner == "schema" {
			input["grammar"] = `root ::= "\"grammar\""`
		}
		if winner == "schema" {
			input["json_schema"] = model.D{"type": "string", "enum": []string{"schema"}}
		}
		table = append(table, apitest.Table{
			Name: "format-precedence-" + winner, URL: "/v1/responses", Token: tokens["responses"],
			Method: http.MethodPost, StatusCode: http.StatusOK, Input: input, GotResp: &kronk.ResponseResponse{},
			CmpFunc: func(got any, _ any) string {
				r := got.(*kronk.ResponseResponse)
				if r.Status != "completed" || len(r.Output) != 1 || len(r.Output[0].Content) != 1 {
					return "expected one completed answer"
				}
				var value string
				if err := json.Unmarshal([]byte(r.Output[0].Content[0].Text), &value); err != nil {
					return "expected a JSON string: " + err.Error()
				}
				if value != winner {
					return fmt.Sprintf("format precedence: got %q, want %q", value, winner)
				}
				return ""
			},
		})
	}
	return table
}

// respImageQwen35 returns response tests for the Qwen3.5 vision model.
func respImageQwen35(t *testing.T, tokens map[string]string) []apitest.Table {
	image, err := readFile(imageFile)
	if err != nil {
		t.Fatalf("read image: %s", err)
	}

	imageURL := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(image)

	return []apitest.Table{
		{
			Name:       "image-good-token",
			URL:        "/v1/responses",
			Token:      tokens["responses"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": visionModelID,
				"input": []model.D{
					{
						"role": "user",
						"content": []model.D{
							{"type": "input_image", "image_url": imageURL},
							{"type": "input_text", "text": "what's in the picture"},
						},
					},
				},
				"max_tokens":           2048,
				"temperature":          0.7,
				"top_p":                0.9,
				"top_k":                40,
				"chat_template_kwargs": model.D{"enable_thinking": false},
			},
			GotResp: &kronk.ResponseResponse{},
			ExpResp: &kronk.ResponseResponse{
				Object:          "response",
				Status:          "completed",
				MaxOutputTokens: new(2048),
				Model:           visionModelID,
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(kronk.ResponseResponse{}, "ID", "CreatedAt", "CompletedAt", "Usage", "Output", "Temperature", "TopP", "ToolChoice", "Truncation", "Tools", "Metadata", "Text", "Reasoning", "ParallelToolCall", "Store"),
				)

				if diff != "" {
					return diff
				}

				return validateRespResponse(got).
					hasValidID().
					hasCreatedAt().
					hasStatus("completed").
					hasOutput().
					hasOutputText().
					warnContainsInContent("giraffes").
					result(t)
			},
		},
	}
}

// respAudioQwen25Omni returns response tests for Qwen2.5-Omni-3B-Q4_K_M model (audio).
func respAudioQwen25Omni(t *testing.T, tokens map[string]string) []apitest.Table {
	audio, err := readFile(audioFile)
	if err != nil {
		t.Fatalf("read audio: %s", err)
	}

	return []apitest.Table{
		{
			Name:       "audio-good-token",
			URL:        "/v1/responses",
			Token:      tokens["responses"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model":       "ggml-org/Qwen2.5-Omni-3B-Q4_K_M",
				"input":       model.AudioMessage("please describe if you hear speech or not in this clip.", audio, "wav"),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
			},
			GotResp: &kronk.ResponseResponse{},
			ExpResp: &kronk.ResponseResponse{
				Object:          "response",
				Status:          "completed",
				MaxOutputTokens: new(2048),
				Model:           "ggml-org/Qwen2.5-Omni-3B-Q4_K_M",
			},
			CmpFunc: func(got any, exp any) string {
				diff := cmp.Diff(got, exp,
					cmpopts.IgnoreFields(kronk.ResponseResponse{}, "ID", "CreatedAt", "CompletedAt", "Usage", "Output", "Temperature", "TopP", "ToolChoice", "Truncation", "Tools", "Metadata", "Text", "Reasoning", "ParallelToolCall", "Store"),
				)

				if diff != "" {
					return diff
				}

				return validateRespResponse(got).
					hasValidID().
					hasCreatedAt().
					hasStatus("completed").
					hasOutput().
					hasOutputText().
					warnContainsInContent("speech").
					result(t)
			},
		},
	}
}

// respStreamQwen3 returns streaming response tests for the Qwen3-1.7B model.
func respStreamQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	return []apitest.Table{
		{
			Name:       "good-token",
			URL:        "/v1/responses",
			Token:      tokens["responses"],
			Method:     http.MethodPost,
			StatusCode: http.StatusOK,
			Input: model.D{
				"model": qwen3ModelID,
				"input": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
				"max_tokens":  2048,
				"temperature": 0.7,
				"top_p":       0.9,
				"top_k":       40,
				"stream":      true,
				"stream_options": model.D{
					"include_usage": true,
				},
			},
			SSECmpFunc: validateRespStream,
			GotResp:    &kronk.ResponseStreamEvent{},
			ExpResp: &kronk.ResponseResponse{
				Object:          "response",
				Status:          "completed",
				MaxOutputTokens: new(2048),
				Model:           qwen3ModelID,
			},
			CmpFunc: func(got any, exp any) string {
				event := got.(*kronk.ResponseStreamEvent)
				if event.Response == nil {
					return "expected response.completed event with Response field"
				}

				diff := cmp.Diff(event.Response, exp,
					cmpopts.IgnoreFields(kronk.ResponseResponse{}, "ID", "CreatedAt", "CompletedAt", "Usage", "Output", "Temperature", "TopP", "ToolChoice", "Truncation", "Tools", "Metadata", "Text", "Reasoning", "ParallelToolCall", "Store"),
				)

				if diff != "" {
					return diff
				}

				return validateRespResponse(event.Response).
					hasValidID().
					hasCreatedAt().
					hasStatus("completed").
					hasOutput().
					hasOutputText().
					warnContainsInContent("gorilla").
					result(t)
			},
		},
	}
}

// =============================================================================

func respEndpoint403(tokens map[string]string) []apitest.Table {
	table := []apitest.Table{
		{
			Name:       "bad-token",
			URL:        "/v1/responses",
			Token:      tokens["embeddings"],
			Method:     http.MethodPost,
			StatusCode: http.StatusForbidden,
			Input: model.D{
				"model": qwen3ModelID,
				"input": model.DocumentArray(
					model.TextMessage(model.RoleUser, "Echo back the word: Gorilla"),
				),
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
