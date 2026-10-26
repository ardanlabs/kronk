package provider

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/tmprl/stages/shared/model"
)

func TestHTTPCompleter(t *testing.T) {
	for _, mode := range []string{"success", "truncated", "content_filter", "tool_calls", "missing-finish", "provider-error", "empty"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var wire struct {
					Model     string          `json:"model"`
					MaxTokens int             `json:"max_tokens"`
					Seed      uint32          `json:"seed"`
					Messages  []model.Message `json:"messages"`
					Format    struct {
						Type   string `json:"type"`
						Schema struct {
							Strict bool           `json:"strict"`
							Value  map[string]any `json:"schema"`
						} `json:"json_schema"`
					} `json:"response_format"`
				}
				if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
					t.Error(err)
				}
				if r.Method != "POST" || wire.Model != "preparer" || wire.MaxTokens != 73 || wire.Seed != 19 || len(wire.Messages) != 1 || wire.Messages[0].Content != "source" || wire.Format.Type != "json_schema" || !wire.Format.Schema.Strict || wire.Format.Schema.Value["type"] != "object" {
					t.Errorf("request: %+v", wire)
				}
				switch mode {
				case "provider-error":
					w.WriteHeader(500)
					fmt.Fprint(w, "secret source content")
				case "empty":
					fmt.Fprint(w, `{"choices":[]}`)
				default:
					finish := "stop"
					if mode == "truncated" {
						finish = "length"
					} else if mode == "content_filter" || mode == "tool_calls" {
						finish = mode
					} else if mode == "missing-finish" {
						finish = ""
					}
					fmt.Fprintf(w, `{"choices":[{"message":{"content":"generated"},"finish_reason":%q}],"usage":{"prompt_tokens":31,"completion_tokens":7}}`, finish)
				}
			}))
			defer server.Close()
			c, err := NewHTTPCompleter(server.URL, server.Client())
			if err != nil {
				t.Fatal(err)
			}
			result, err := c.Complete(t.Context(), model.CompletionRequest{Model: "preparer", MaxTokens: 73, Seed: 19, Messages: []model.Message{{Role: "user", Content: "source"}}, ResponseSchema: map[string]any{"type": "object"}})
			switch mode {
			case "success":
				if err != nil || result.Content != "generated" || result.PromptTokens != 31 || result.CompletionTokens != 7 {
					t.Fatalf("completion: %+v %v", result, err)
				}
			case "truncated":
				if !errors.Is(err, model.ErrCompletionTruncated) {
					t.Fatalf("truncation: %v", err)
				}
			default:
				if err == nil || strings.Contains(err.Error(), "secret source") {
					t.Fatalf("invalid/error response: %v", err)
				}
			}
		})
	}
}
