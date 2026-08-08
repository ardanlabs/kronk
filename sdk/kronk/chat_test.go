package kronk

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

func TestStreamIncludeUsage(t *testing.T) {
	tests := []struct {
		name       string
		d          model.D
		defaultVal bool
		want       bool
	}{
		{name: "omitted defaults false", d: model.D{}, want: false},
		{name: "omitted uses configured default", d: model.D{}, defaultVal: true, want: true},
		{name: "empty options defaults false", d: model.D{"stream_options": model.D{}}, want: false},
		{name: "empty options uses configured default", d: model.D{"stream_options": model.D{}}, defaultVal: true, want: true},
		{name: "D true", d: model.D{"stream_options": model.D{"include_usage": true}}, want: true},
		{name: "D false overrides configured default", d: model.D{"stream_options": model.D{"include_usage": false}}, defaultVal: true, want: false},
		{name: "map true", d: model.D{"stream_options": map[string]any{"include_usage": true}}, want: true},
		{name: "map false overrides configured default", d: model.D{"stream_options": map[string]any{"include_usage": false}}, defaultVal: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := streamIncludeUsage(tt.d, tt.defaultVal); got != tt.want {
				t.Errorf("streamIncludeUsage: got %t, want %t", got, tt.want)
			}
		})
	}
}

func TestMarshalChatStreamError(t *testing.T) {
	resp := model.ChatResponseErr(
		"id",
		model.ObjectChatText,
		"model",
		0,
		errors.New("inference failed"),
		model.Usage{},
	)

	data, err := marshalChatStreamError(resp)
	if err != nil {
		t.Fatalf("marshalChatStreamError: %v", err)
	}

	var wire map[string]json.RawMessage
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got, want := len(wire), 1; got != want {
		t.Fatalf("top-level fields: got %d, want %d", got, want)
	}

	var apiErr struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(wire["error"], &apiErr); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if got, want := apiErr.Message, "inference failed"; got != want {
		t.Errorf("Message: got %q, want %q", got, want)
	}
	if got, want := apiErr.Type, "server_error"; got != want {
		t.Errorf("Type: got %q, want %q", got, want)
	}
	if got, want := apiErr.Code, "server_error"; got != want {
		t.Errorf("Code: got %q, want %q", got, want)
	}
}

func TestMarshalChatStreamResponseUsage(t *testing.T) {
	u := model.Usage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	resp := model.ChatResponse{
		ID:                "chatcmpl-id",
		Object:            model.ObjectChatText,
		Created:           123,
		Model:             "model",
		SystemFingerprint: "fp_kronk",
		Choices:           []model.Choice{{Index: 0}},
		Usage:             &u,
	}

	tests := []struct {
		name             string
		resp             model.ChatResponse
		includeUsage     bool
		wantUsageJSON    string
		wantUsagePresent bool
		wantChoices      int
	}{
		{name: "omitted", resp: resp, wantChoices: 1},
		{name: "ordinary chunk null", resp: func() model.ChatResponse {
			resp := resp
			resp.Usage = nil
			return resp
		}(), includeUsage: true, wantUsageJSON: "null", wantChoices: 1},
		{name: "usage chunk", resp: func() model.ChatResponse {
			resp := resp
			resp.Choices = []model.Choice{}
			return resp
		}(), includeUsage: true, wantUsagePresent: true, wantChoices: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := marshalChatStreamResponse(tt.resp, tt.includeUsage)
			if err != nil {
				t.Fatalf("marshalChatStreamResponse: %v", err)
			}

			var wire struct {
				Choices []model.Choice  `json:"choices"`
				Usage   json.RawMessage `json:"usage"`
			}
			if err := json.Unmarshal(data, &wire); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got := len(wire.Choices); got != tt.wantChoices {
				t.Errorf("choices: got %d, want %d", got, tt.wantChoices)
			}
			gotUsage := string(wire.Usage)
			if !tt.wantUsagePresent && gotUsage != tt.wantUsageJSON {
				t.Errorf("usage: got %s, want %s", gotUsage, tt.wantUsageJSON)
			}
			if tt.wantUsagePresent {
				var got model.Usage
				if err := json.Unmarshal(wire.Usage, &got); err != nil {
					t.Fatalf("Unmarshal usage: %v", err)
				}
				if got.TotalTokens != u.TotalTokens {
					t.Errorf("TotalTokens: got %d, want %d", got.TotalTokens, u.TotalTokens)
				}
			}
		})
	}
}

func TestChatValidatesRequestBeforeAdmission(t *testing.T) {
	tests := []struct {
		name string
		d    model.D
		want error
	}{
		{name: "missing", d: model.D{}, want: model.ErrMessagesMissing},
		{name: "invalid", d: model.D{"messages": "invalid"}, want: model.ErrMessagesInvalid},
		{
			name: "required without tools",
			d: model.D{
				"messages":    []model.D{{"role": "user", "content": "hello"}},
				"tool_choice": "required",
			},
			want: model.ErrInvalidRequest,
		},
		{
			name: "chat template kwargs non-object",
			d: model.D{
				"messages":             []model.D{{"role": "user", "content": "hello"}},
				"chat_template_kwargs": false,
			},
			want: model.ErrInvalidRequest,
		},
		{
			name: "chat template kwargs self-reference",
			d: model.D{
				"messages": []model.D{{"role": "user", "content": "hello"}},
				"chat_template_kwargs": model.D{
					"chat_template_kwargs": model.D{},
				},
			},
			want: model.ErrInvalidRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var krn Kronk

			if _, err := krn.Chat(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("Chat: got %v, want %v", err, tt.want)
			}
			if _, err := krn.ChatStreaming(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("ChatStreaming: got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestStreamingStartupErrorReleasesAdmission(t *testing.T) {
	startupErr := errors.New("startup failed")
	krn := Kronk{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Second)),
		model:       &model.Model{},
		admissionCh: make(chan struct{}, 1),
	}

	ch, err := streaming(t.Context(), &krn,
		func(*model.Model) (<-chan model.ChatResponse, error) { return nil, startupErr },
		func(err error) model.ChatResponse { return model.ChatResponse{} },
	)
	if !errors.Is(err, startupErr) {
		t.Fatalf("streaming: got %v, want startup error", err)
	}
	if ch != nil {
		t.Fatal("streaming: got channel, want nil")
	}
	if got := len(krn.admissionCh); got != 0 {
		t.Fatalf("admission permits: got %d, want 0", got)
	}
	if got := krn.activeStreams.Load(); got != 0 {
		t.Fatalf("active streams: got %d, want 0", got)
	}
}

func TestStreamingWithDoesNotEmitStartEventsOnStartupError(t *testing.T) {
	startupErr := errors.New("startup failed")
	krn := Kronk{
		cfg:         model.NewConfig(model.WithAdmissionTimeout(time.Second)),
		model:       &model.Model{},
		admissionCh: make(chan struct{}, 1),
	}
	started := false

	ch, err := streamingWith(t.Context(), &krn,
		func(*model.Model) (<-chan model.ChatResponse, error) { return nil, startupErr },
		streamProcessor[model.ChatResponse, string]{
			Start: func() []string {
				started = true
				return []string{"started"}
			},
		},
		func(error) string { return "error" },
	)
	if !errors.Is(err, startupErr) {
		t.Fatalf("streamingWith: got %v, want startup error", err)
	}
	if ch != nil {
		t.Fatal("streamingWith: got channel, want nil")
	}
	if started {
		t.Fatal("streamingWith: emitted start events before successful startup")
	}
	if got := len(krn.admissionCh); got != 0 {
		t.Fatalf("admission permits: got %d, want 0", got)
	}
}

func TestWriteAndFlush(t *testing.T) {
	errWrite := errors.New("write failed")
	errFlush := errors.New("flush failed")

	tests := []struct {
		name      string
		writeErr  error
		flushErr  error
		wantErr   error
		wantFlush int
	}{
		{name: "success", wantFlush: 1},
		{name: "write failure", writeErr: errWrite, wantErr: errWrite},
		{name: "flush failure", flushErr: errFlush, wantErr: errFlush, wantFlush: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := streamResponseWriter{
				header:   make(http.Header),
				writeErr: tt.writeErr,
				flushErr: tt.flushErr,
			}

			err := writeAndFlush(&w, []byte("event"))
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("writeAndFlush error: got %v, want %v", err, tt.wantErr)
			}
			if got := w.flushes; got != tt.wantFlush {
				t.Errorf("flushes: got %d, want %d", got, tt.wantFlush)
			}
		})
	}
}

func TestSupportsResponseFlush(t *testing.T) {
	w := flushErrorResponseWriter{header: make(http.Header)}
	if !supportsResponseFlush(&w) {
		t.Fatal("FlushError writer reported unsupported")
	}

	wrapped := unwrapResponseWriter{
		ResponseWriter: &w,
		header:         make(http.Header),
	}
	if !supportsResponseFlush(&wrapped) {
		t.Fatal("wrapped FlushError writer reported unsupported")
	}
}

type streamResponseWriter struct {
	header   http.Header
	writeErr error
	flushErr error
	flushes  int
}

func (w *streamResponseWriter) Header() http.Header {
	return w.header
}

func (w *streamResponseWriter) Write(data []byte) (int, error) {
	if w.writeErr != nil {
		return 0, w.writeErr
	}
	return len(data), nil
}

func (w *streamResponseWriter) WriteHeader(int) {}

func (w *streamResponseWriter) Flush() {
	w.flushes++
}

func (w *streamResponseWriter) FlushError() error {
	w.flushes++
	return w.flushErr
}

type flushErrorResponseWriter struct {
	header http.Header
}

func (w *flushErrorResponseWriter) Header() http.Header {
	return w.header
}

func (w *flushErrorResponseWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (w *flushErrorResponseWriter) WriteHeader(int) {}

func (w *flushErrorResponseWriter) FlushError() error {
	return nil
}

type unwrapResponseWriter struct {
	http.ResponseWriter
	header http.Header
}

func (w *unwrapResponseWriter) Header() http.Header {
	return w.header
}

func (w *unwrapResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
