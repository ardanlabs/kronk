package msgsapp

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/mid"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/security/auth"
	"github.com/ardanlabs/kronk/cmd/server/foundation/logger"
	"github.com/ardanlabs/kronk/cmd/server/foundation/web"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestMessagesRejectsMissingMessagesBeforeModelAcquisition(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"test","max_tokens":1}`))

	resp := (&app{}).messages(t.Context(), req)
	appErr, ok := resp.(*errs.Error)
	if !ok {
		t.Fatalf("messages: got %T, want *errs.Error", resp)
	}
	if !appErr.Code.Equal(errs.InvalidArgument) {
		t.Errorf("Code: got %s, want %s", appErr.Code, errs.InvalidArgument)
	}
}

func TestMessagesRejectsStopSequencesBeforeModelAcquisition(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"stop_sequences":["END"]}`))

	resp := (&app{}).messages(t.Context(), req)
	appErr, ok := resp.(*errs.Error)
	if !ok {
		t.Fatalf("messages: got %T, want *errs.Error", resp)
	}
	if !appErr.Code.Equal(errs.InvalidArgument) {
		t.Errorf("Code: got %s, want %s", appErr.Code, errs.InvalidArgument)
	}
	if got, want := appErr.Message, "stop_sequences is not supported"; got != want {
		t.Errorf("Message: got %q, want %q", got, want)
	}
}

func TestStreamStateSendEventReportsTransportErrors(t *testing.T) {
	errWrite := errors.New("write failed")
	errFlush := errors.New("flush failed")

	tests := []struct {
		name     string
		writeErr error
		flushErr error
		wantErr  error
	}{
		{name: "success"},
		{name: "write failure", writeErr: errWrite, wantErr: errWrite},
		{name: "flush failure", flushErr: errFlush, wantErr: errFlush},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := eventResponseWriter{
				header:   make(http.Header),
				writeErr: tt.writeErr,
				flushErr: tt.flushErr,
			}
			state := streamState{w: &w}

			err := state.sendEvent("message_stop", map[string]string{"type": "message_stop"})
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("sendEvent error: got %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestStreamStateConvertsSDKErrorChunkToAnthropicError(t *testing.T) {
	rr := httptest.NewRecorder()
	state := streamState{w: rr}
	resp := model.ChatResponseErr("request-id", "chat.completion.chunk", "model", 0, errors.New("sensitive internal path"), model.Usage{})

	if err := state.processChunk(resp); err != nil {
		t.Fatalf("processChunk: %v", err)
	}
	if !state.failed {
		t.Fatal("failed: got false, want true")
	}
	if state.started {
		t.Fatal("started: got true, want false")
	}
	if got := rr.Body.String(); !strings.Contains(got, "event: error") || !strings.Contains(got, "Internal Server Error") {
		t.Errorf("error event: got %q", got)
	}
	if strings.Contains(rr.Body.String(), "sensitive internal path") {
		t.Errorf("error event exposed SDK error: %q", rr.Body.String())
	}
	if err := state.finish(); err != nil {
		t.Fatalf("finish after error: %v", err)
	}
}

func TestStreamStateRejectsEmptyStream(t *testing.T) {
	state := streamState{w: httptest.NewRecorder()}

	if err := state.finish(); err == nil {
		t.Fatal("finish: got nil, want missing message_start error")
	}
}

type eventResponseWriter struct {
	header   http.Header
	writeErr error
	flushErr error
}

func (w *eventResponseWriter) Header() http.Header {
	return w.header
}

func (w *eventResponseWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(data), nil
}

func (w *eventResponseWriter) WriteHeader(int) {}

func (w *eventResponseWriter) FlushError() error {
	return w.flushErr
}

func TestToOpenAIMaxTokens(t *testing.T) {
	d := toOpenAI(MessagesRequest{MaxTokens: 32})

	if got := d["max_tokens"]; got != 32 {
		t.Errorf("max_tokens: got %v, want 32", got)
	}
}

func TestToOpenAIForwardsTopK(t *testing.T) {
	topK := 17
	d := toOpenAI(MessagesRequest{TopK: &topK})

	if got := d["top_k"]; got != topK {
		t.Errorf("top_k: got %v, want %d", got, topK)
	}
}

func TestToMessagesResponseToolInputIsObject(t *testing.T) {
	resp := model.ChatResponse{
		Choices: []model.Choice{
			{
				Message: &model.ResponseMessage{
					ToolCalls: []model.ResponseToolCall{
						{
							ID: "call-1",
							Function: model.ResponseToolCallFunction{
								Name: "bash",
								Arguments: model.ToolCallArguments{
									"command": "echo hello",
								},
							},
						},
					},
				},
			},
		},
	}

	data, err := json.Marshal(toMessagesResponse(resp))
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	var got struct {
		Content []struct {
			Input map[string]any `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if got, want := got.Content[0].Input["command"], "echo hello"; got != want {
		t.Errorf("input command: got %v, want %v", got, want)
	}
}

func TestToAnthropicStopReason(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "stop", input: model.FinishReasonStop, want: "end_turn"},
		{name: "tool call", input: model.FinishReasonTool, want: "tool_use"},
		{name: "token limit", input: model.FinishReasonLength, want: "max_tokens"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toAnthropicStopReason(tt.input); got != tt.want {
				t.Errorf("toAnthropicStopReason(%q): got %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestMessagesSDKStreamWireProtocol(t *testing.T) {
	calls := []model.ResponseToolCall{
		{ID: "call-a", Function: model.ResponseToolCallFunction{Name: "lookup", Arguments: model.ToolCallArguments{"id": json.Number("9007199254740993")}}},
		{ID: "call-b", Function: model.ResponseToolCallFunction{Name: "lookup", Arguments: model.ToolCallArguments{}}},
	}
	for _, tt := range []struct {
		name        string
		text        string
		calls       []model.ResponseToolCall
		incremental bool
		reason      string
		wantStop    string
	}{
		{"text", "Hello world", nil, false, model.FinishReasonStop, "end_turn"},
		{"buffered tools", "", calls, false, model.FinishReasonTool, "tool_use"},
		{"incremental text and tools", "Checking now", calls, true, model.FinishReasonTool, "tool_use"},
		{"token limit", "Checking", calls[:1], true, model.FinishReasonLength, "max_tokens"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			state := streamState{w: rr}
			chunk := func(choice model.Choice) {
				t.Helper()
				if err := state.processChunk(model.ChatResponse{ID: "msg-id", Model: "test-model", Choices: []model.Choice{choice}}); err != nil {
					t.Fatal(err)
				}
			}
			if tt.text != "" {
				chunk(model.Choice{Delta: &model.ResponseMessage{Content: tt.text[:2]}})
				chunk(model.Choice{Delta: &model.ResponseMessage{Content: tt.text[2:]}})
			}
			for i, call := range tt.calls {
				if tt.incremental {
					chunk(model.Choice{Delta: &model.ResponseMessage{ToolCallDeltas: []model.ResponseToolCallDelta{{ID: call.ID, Index: i, Function: model.ResponseToolCallDeltaFunction{Name: call.Function.Name}}}}})
				}
				args, err := json.Marshal(map[string]any(call.Function.Arguments))
				if err != nil {
					t.Fatal(err)
				}
				// Completion deltas contain full JSON, not append-only fragments.
				chunk(model.Choice{Delta: &model.ResponseMessage{ToolCallDeltas: []model.ResponseToolCallDelta{{Index: i, Function: model.ResponseToolCallDeltaFunction{Arguments: string(args)}}}}})
			}
			chunk(model.Choice{Delta: &model.ResponseMessage{}, Message: &model.ResponseMessage{Content: tt.text, ToolCalls: tt.calls}, FinishReasonPtr: &tt.reason})
			if err := state.processChunk(model.ChatResponse{Usage: &model.Usage{PromptTokens: 23, CompletionTokens: 7}}); err != nil {
				t.Fatal(err)
			}
			if err := state.finish(); err != nil {
				t.Fatal(err)
			}

			var text, stop string
			var tools []struct{ id, name, args string }
			active, starts, stops, messageStops := -1, 0, 0, 0
			for record := range strings.SplitSeq(strings.TrimSpace(rr.Body.String()), "\n\n") {
				name, data, ok := strings.Cut(record, "\ndata: ")
				if !ok {
					t.Fatalf("malformed SSE: %q", record)
				}
				var event struct {
					Type         string `json:"type"`
					Index        int    `json:"index"`
					ContentBlock struct {
						Type  string          `json:"type"`
						Text  *string         `json:"text"`
						ID    string          `json:"id"`
						Name  string          `json:"name"`
						Input json.RawMessage `json:"input"`
					} `json:"content_block"`
					Delta struct {
						Text        string `json:"text"`
						PartialJSON string `json:"partial_json"`
						StopReason  string `json:"stop_reason"`
					} `json:"delta"`
					Usage DeltaUsage `json:"usage"`
				}
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					t.Fatal(err)
				}
				if name != "event: "+event.Type {
					t.Fatalf("event name mismatch: %q %s", name, data)
				}
				switch event.Type {
				case "content_block_start":
					if active != -1 || event.Index != starts {
						t.Fatalf("block start: got index %d active %d, want %d inactive", event.Index, active, starts)
					}
					starts++
					active = event.Index
					if event.ContentBlock.Type == "text" {
						if event.ContentBlock.Text == nil || *event.ContentBlock.Text != "" {
							t.Fatalf("text block must include text:\"\": %s", data)
						}
					} else {
						if event.ContentBlock.Text != nil || string(event.ContentBlock.Input) != "{}" {
							t.Fatalf("tool start must include input:{} and no text: %s", data)
						}
						tools = append(tools, struct{ id, name, args string }{id: event.ContentBlock.ID, name: event.ContentBlock.Name})
					}
				case "content_block_delta":
					if active != event.Index || active == -1 {
						t.Fatalf("delta outside block: %s", data)
					}
					text += event.Delta.Text
					if event.Delta.PartialJSON != "" {
						tools[len(tools)-1].args += event.Delta.PartialJSON
					}
				case "content_block_stop":
					if active != event.Index || active == -1 {
						t.Fatalf("stop outside block: %s", data)
					}
					active = -1
					stops++
				case "message_delta":
					stop = event.Delta.StopReason
					if event.Usage.InputTokens != 23 || event.Usage.OutputTokens != 7 {
						t.Errorf("final usage: got %+v, want 23/7", event.Usage)
					}
				case "message_stop":
					messageStops++
				}
			}
			if text != tt.text || stop != tt.wantStop || starts != stops || messageStops != 1 || len(tools) != len(tt.calls) {
				t.Fatalf("stream: text=%q stop=%q starts/stops=%d/%d messageStops=%d tools=%+v", text, stop, starts, stops, messageStops, tools)
			}
			for i, tool := range tools {
				args, err := json.Marshal(map[string]any(tt.calls[i].Function.Arguments))
				if err != nil {
					t.Fatal(err)
				}
				if tool.id != tt.calls[i].ID || tool.name != "lookup" || tool.args != string(args) {
					t.Errorf("tool %d: got %+v, want %s lookup %s", i, tool, tt.calls[i].ID, args)
				}
			}
		})
	}
}

func TestMessagesRejectsInvalidBodiesThroughRoute(t *testing.T) {
	log := logger.New(io.Discard, logger.LevelError, "TEST", nil)
	server := web.NewApp(log.Info, mid.Errors(log), mid.Panics())
	Routes(server, Config{Log: log, AuthorizationMode: auth.Open, InferenceTimeout: time.Minute})
	for _, stream := range []string{"false", "true"} {
		for _, body := range []string{
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"stream":` + stream + `}{"second":true}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"stream":` + stream + `}42`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"stream":` + stream + `} trailing-garbage`,
			`{"model":"test","max_tokens":-1,"messages":[{"role":"user","content":"hello"}],"stream":` + stream + `}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"system","content":"hello"}],"stream":` + stream + `}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":{}}],"stream":` + stream + `}`,
			`{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hello"}],"system":42,"stream":` + stream + `}`,
			`{"max_tokens":1,"stream":` + stream + `} ` + "\n\t",
		} {
			rr := httptest.NewRecorder()
			server.ServeHTTP(rr, httptest.NewRequest("POST", "/v1/messages", strings.NewReader(body)))
			if rr.Code != http.StatusBadRequest || rr.Header().Get("Content-Type") != "application/json" {
				t.Fatalf("invalid request %s: got %d %s", body, rr.Code, rr.Body)
			}
			var got ErrorResponse
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Type != "error" || got.Error.Type != "invalid_request_error" || got.Error.Message == "" {
				t.Errorf("invalid request %s: got %+v", body, got)
			}
			if id := rr.Header().Get("request-id"); id == "" || id != rr.Header().Get("anthropic-request-id") {
				t.Errorf("request ID headers: got %v", rr.Header())
			}
		}
	}
}
