package msgsapp

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/errs"
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
