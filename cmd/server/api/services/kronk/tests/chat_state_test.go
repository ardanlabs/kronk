package chatapi_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// chatStreamState exercises rendered-prefix identity through the HTTP route.
// There is no cache_id request contract: IMC identifies rendered token prefixes.
func chatStreamState(t *testing.T, test *apitest.Test, token, modelID string, enabled bool) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	krn, err := test.Pool.Kronk.AquireModel(ctx, modelID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := krn.ModelConfig()
	t.Logf("effective configuration for %s: %s", modelID, cfg.String())
	if cfg.NSeqMax() != 1 || cfg.ContextWindow() != 8192 || cfg.IncrementalCache() != enabled ||
		cfg.CacheMinTokens() != 1 || cfg.Speculation != model.SpeculationDisabled ||
		cfg.CacheTypeK != model.GGMLTypeF16 || cfg.CacheTypeV != model.GGMLTypeF16 {
		t.Fatal("effective model configuration does not match the API state fixture")
	}

	run := func(name string, request model.D, want string, reuse bool) string {
		t.Helper()
		var answer string
		test.RunStreaming(t, []apitest.Table{{
			Name: name, URL: "/v1/chat/completions", Token: token,
			Method: http.MethodPost, StatusCode: http.StatusOK, Input: request,
			GotResp: &model.ChatResponse{}, RequireDone: true, StreamResponseOffset: 1,
			CmpFunc: func(got, _ any) string {
				if resp := got.(*model.ChatResponse); resp.Model != modelID {
					return fmt.Sprintf("model: got %q, want %q", resp.Model, modelID)
				}
				return ""
			},
			StreamCmpFunc: func(events []json.RawMessage) string {
				if diff := validateChatUsageStreamReasoning(events, false); diff != "" {
					return diff
				}
				var terminal, footer model.ChatResponse
				if err := json.Unmarshal(events[len(events)-2], &terminal); err != nil {
					return err.Error()
				}
				if err := json.Unmarshal(events[len(events)-1], &footer); err != nil {
					return err.Error()
				}
				var text strings.Builder
				for _, data := range events {
					var chunk model.ChatResponse
					if err := json.Unmarshal(data, &chunk); err != nil {
						return err.Error()
					}
					if chunk.ID != terminal.ID || chunk.Model != modelID {
						return "stream identity changed"
					}
					for _, choice := range chunk.Choices {
						if choice.Delta != nil {
							text.WriteString(choice.Delta.Content)
						}
					}
				}
				terminal.Usage = footer.Usage
				if err := testlib.CheckStateReply(terminal, text.String(), want); err != nil {
					return err.Error()
				}
				cached := footer.Usage.PromptTokensDetails.CachedTokens
				if !enabled && cached != 0 {
					return fmt.Sprintf("IMC off: got %d cached tokens, want 0", cached)
				}
				if enabled && reuse && cached == 0 {
					return "qualified replay/append did not reuse cached tokens"
				}
				t.Logf("%s: answer=%q cached=%d", name, text.String(), cached)
				answer = text.String()
				return ""
			},
		}}, "state")
		if t.Failed() {
			t.FailNow() // Do not fabricate history after a failed preceding turn.
		}
		return answer
	}

	initial := testlib.StateRequest("ALPHA", "DELTA", 101)
	initial["model"], initial["stream"] = modelID, true
	first := run("initial", initial, "ALPHA", false)
	run("exact-replay", initial, "ALPHA", true)
	extended := initial.Clone()
	extended["messages"] = append(extended["messages"].([]model.D),
		model.D{"role": "assistant", "content": first},
		model.D{"role": "user", "content": "What is your second code word?"},
	)
	second := run("append-actual-output", extended, "DELTA", true)
	diverged := extended.Clone()
	messages := diverged["messages"].([]model.D)
	messages[0]["content"] = strings.Replace(messages[0]["content"].(string), "ALPHA", "COPPER", 1)
	messages[1]["content"] = "What is your second code word?"
	messages[2]["content"] = second
	messages[3]["content"] = "What is your first code word?"
	run("divergent-prefix", diverged, "COPPER", false)
	independent := testlib.StateRequest("BRAVO", "ECHO", 202)
	independent["model"], independent["stream"] = modelID, true
	run("independent-session", independent, "BRAVO", false)
	run("original-after-session-switch", initial, "ALPHA", false)
}

func chatStreamOptionsQwen3(tokens map[string]string) []apitest.Table {
	var tables []apitest.Table
	for _, includeUsage := range []bool{false, true} {
		for _, limited := range []bool{false, true} {
			request := testlib.StateRequest("ALPHA", "DELTA", 101)
			request["model"], request["stream"] = qwen3ModelID, true
			request["stream_options"] = model.D{"include_usage": includeUsage}
			finish := model.FinishReasonStop
			if limited {
				request["messages"] = []model.D{{"role": "user", "content": "Count from one to one hundred, writing every number on its own line."}}
				request["max_tokens"] = 1
				finish = model.FinishReasonLength
			}
			offset := 0
			if includeUsage {
				offset = 1
			}
			tables = append(tables, apitest.Table{
				Name: fmt.Sprintf("usage-%t-length-%t", includeUsage, limited),
				URL:  "/v1/chat/completions", Token: tokens["chat-completions"],
				Method: http.MethodPost, StatusCode: http.StatusOK, Input: request,
				GotResp: &model.ChatResponse{}, StreamResponseOffset: offset, RequireDone: true,
				CmpFunc: func(got, _ any) string {
					resp := got.(*model.ChatResponse)
					if len(resp.Choices) != 1 || resp.Choices[0].FinishReason() != finish {
						return fmt.Sprintf("expected one terminal choice with finish_reason %q", finish)
					}
					return ""
				},
				StreamCmpFunc: func(events []json.RawMessage) string {
					return validateChatStreamOptions(events, includeUsage)
				},
			})
		}
	}
	return tables
}

func validateChatStreamOptions(events []json.RawMessage, includeUsage bool) string {
	if includeUsage {
		return validateChatUsageStreamReasoning(events, false)
	}
	if len(events) == 0 {
		return "missing chat stream"
	}
	var id string
	for i, data := range events {
		var chunk model.ChatResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			return err.Error()
		}
		if chunk.Usage != nil || len(chunk.Choices) != 1 {
			return "include_usage=false requires one choice and no usage in every chunk"
		}
		if i == 0 {
			id = chunk.ID
		}
		if id == "" || chunk.ID != id || chunk.Model != qwen3ModelID {
			return "invalid or changed stream identity"
		}
		finished := chunk.Choices[0].FinishReason() != ""
		if finished != (i == len(events)-1) {
			return "only the final choice may have a finish reason"
		}
	}
	return ""
}

func TestChatStreamOptions(t *testing.T) {
	delta := model.ChatResponse{ID: "completion", Model: qwen3ModelID,
		Choices: []model.Choice{{Delta: &model.ResponseMessage{Content: "A"}}}}
	terminal := model.ChatResponse{ID: "completion", Model: qwen3ModelID,
		Choices: []model.Choice{{FinishReasonPtr: new(model.FinishReasonLength)}}}
	footer := model.ChatResponse{ID: "completion", Model: qwen3ModelID, Choices: []model.Choice{},
		Usage: &model.Usage{PromptTokens: 20, CompletionTokens: 1, TotalTokens: 21}}
	twoTokenFooter := footer
	twoTokenFooter.Usage = &model.Usage{PromptTokens: 20, CompletionTokens: 2, TotalTokens: 22}
	for _, tc := range []struct {
		name    string
		chunks  []model.ChatResponse
		usage   bool
		wantErr bool
	}{
		{name: "no usage", chunks: []model.ChatResponse{delta, terminal}},
		{name: "one token zero throughput", chunks: []model.ChatResponse{delta, terminal, footer}, usage: true},
		{name: "multiple tokens zero throughput", chunks: []model.ChatResponse{delta, terminal, twoTokenFooter}, usage: true, wantErr: true},
		{name: "missing usage", chunks: []model.ChatResponse{delta, terminal}, usage: true, wantErr: true},
		{name: "unexpected usage", chunks: []model.ChatResponse{delta, terminal, footer}, wantErr: true},
		{name: "missing terminal", chunks: []model.ChatResponse{delta}, wantErr: true},
		{name: "early terminal", chunks: []model.ChatResponse{terminal, delta}, wantErr: true},
		{name: "duplicate terminal", chunks: []model.ChatResponse{terminal, terminal}, wantErr: true},
		{name: "changed identity", chunks: []model.ChatResponse{{ID: "other", Model: qwen3ModelID, Choices: delta.Choices}, terminal}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events []json.RawMessage
			for _, chunk := range tc.chunks {
				data, err := json.Marshal(chunk)
				if err != nil {
					t.Fatal(err)
				}
				if tc.usage && chunk.Usage == nil {
					// The HTTP encoder emits explicit null usage when requested;
					// ChatResponse's ordinary JSON marshal omits nil usage.
					var wire map[string]json.RawMessage
					if err := json.Unmarshal(data, &wire); err != nil {
						t.Fatal(err)
					}
					wire["usage"] = json.RawMessage("null")
					data, err = json.Marshal(wire)
					if err != nil {
						t.Fatal(err)
					}
				}
				events = append(events, data)
			}
			if diff := validateChatStreamOptions(events, tc.usage); (diff != "") != tc.wantErr {
				t.Fatalf("validation: got %q, want error %t", diff, tc.wantErr)
			}
		})
	}
}
