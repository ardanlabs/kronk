package chatapi_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/domain/msgsapp"
	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
)

// msgsStreamResult is the content a client reconstructs, not just the last event.
type msgsStreamResult struct {
	Text       string
	Tools      []msgsapp.ResponseContentBlock
	StopReason string
	Usage      msgsapp.DeltaUsage
	Failed     bool
}

// validateMsgsStream accepts an explicit terminal error only when requested.
// A failed stream can leave a block open, but must never subsequently complete.
func validateMsgsStream(events []apitest.SSEEvent, model string, allowError bool) (msgsStreamResult, string) {
	var result msgsStreamResult
	started, final, terminal := false, false, false
	index, open := 0, false
	var block msgsapp.ContentBlockMetadata
	var text, args strings.Builder
	ids := make(map[string]bool)
	for i, event := range events {
		if terminal {
			return result, fmt.Sprintf("event %d: event after terminal event", i)
		}
		// Decode the shared envelope once. Pointer fields distinguish missing
		// indices/usage from legitimate zero values.
		var e struct {
			Type         string                       `json:"type"`
			Index        *int                         `json:"index"`
			Message      msgsapp.MessageStartMetadata `json:"message"`
			ContentBlock msgsapp.ContentBlockMetadata `json:"content_block"`
			Delta        struct {
				Type         string  `json:"type"`
				Text         string  `json:"text"`
				PartialJSON  string  `json:"partial_json"`
				StopReason   string  `json:"stop_reason"`
				StopSequence *string `json:"stop_sequence"`
			} `json:"delta"`
			Usage *struct {
				InputTokens  *int `json:"input_tokens"`
				OutputTokens *int `json:"output_tokens"`
			} `json:"usage"`
			Error msgsapp.ErrorDetail `json:"error"`
		}
		if err := json.Unmarshal(event.Data, &e); err != nil {
			return result, fmt.Sprintf("event %d: invalid JSON: %s", i, err)
		}
		if event.Event != e.Type || e.Type == "" {
			return result, fmt.Sprintf("event %d: event name/type mismatch", i)
		}
		if e.Type == "error" {
			if !allowError || final || e.Error.Type == "" || e.Error.Message == "" {
				return result, "unexpected or invalid terminal error"
			}
			result.Failed, terminal = true, true
			continue
		}
		if e.Type == "message_start" {
			m := e.Message
			if started || i != 0 || m.ID == "" || m.Model != model || m.Type != "message" || m.Role != "assistant" || len(m.Content) != 0 || m.StopReason != nil || m.StopSequence != nil || m.Usage.InputTokens < 0 || m.Usage.OutputTokens != 0 {
				return result, "invalid message_start"
			}
			started = true
			continue
		}
		if !started {
			return result, "event before message_start"
		}
		if final && e.Type != "message_stop" {
			return result, "expected message_stop after message_delta"
		}
		switch e.Type {
		case "content_block_start":
			if open || e.Index == nil || *e.Index != index {
				return result, "invalid content block start/index"
			}
			block = e.ContentBlock
			text.Reset()
			args.Reset()
			switch block.Type {
			case "text":
				text.WriteString(block.Text)
			case "tool_use":
				if block.ID == "" || block.Name == "" || ids[block.ID] {
					return result, "invalid or duplicate tool identity"
				}
				ids[block.ID] = true
			default:
				return result, "unsupported content block type"
			}
			open = true
		case "content_block_delta", "content_block_stop":
			if !open || e.Index == nil || *e.Index != index {
				return result, "delta/stop for unopened or wrong block"
			}
			if e.Type == "content_block_delta" {
				switch {
				case block.Type == "text" && e.Delta.Type == "text_delta":
					text.WriteString(e.Delta.Text)
				case block.Type == "tool_use" && e.Delta.Type == "input_json_delta":
					args.WriteString(e.Delta.PartialJSON)
				default:
					return result, "delta type does not match block"
				}
				continue
			}
			if block.Type == "text" {
				result.Text += text.String()
			} else {
				var input map[string]any
				if err := json.Unmarshal([]byte(args.String()), &input); err != nil || input == nil {
					return result, "tool arguments are not a JSON object"
				}
				result.Tools = append(result.Tools, msgsapp.ResponseContentBlock{Type: "tool_use", ID: block.ID, Name: block.Name, Input: input})
			}
			open = false
			index++
		case "message_delta":
			if open || e.Usage == nil || e.Usage.InputTokens == nil || e.Usage.OutputTokens == nil || *e.Usage.InputTokens < 0 || *e.Usage.OutputTokens < 0 {
				return result, "invalid final usage or unclosed block"
			}
			switch e.Delta.StopReason {
			case "end_turn", "max_tokens":
			case "tool_use":
				if len(result.Tools) == 0 {
					return result, "tool_use stop without tool content"
				}
			default:
				return result, "unsupported stop reason"
			}
			if e.Delta.StopSequence != nil {
				return result, "unexpected stop sequence"
			}
			result.StopReason = e.Delta.StopReason
			result.Usage = msgsapp.DeltaUsage{InputTokens: *e.Usage.InputTokens, OutputTokens: *e.Usage.OutputTokens}
			final = true
		case "message_stop":
			if !final || open {
				return result, "message_stop before final delta"
			}
			terminal = true
		default:
			return result, "unsupported event type: " + e.Type
		}
	}
	if !terminal {
		return result, "stream ended without terminal event"
	}
	return result, ""
}

// =============================================================================

func TestMsgsStreamValidation(t *testing.T) {
	event := func(name, data string) apitest.SSEEvent {
		return apitest.SSEEvent{Event: name, Data: json.RawMessage(data)}
	}
	start := event("message_start", `{"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"test","content":[],"usage":{"input_tokens":0,"output_tokens":0}}}`)
	block := event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
	delta := event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hello"}}`)
	closeBlock := event("content_block_stop", `{"type":"content_block_stop","index":0}`)
	final := event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"input_tokens":12,"output_tokens":3}}`)
	stop := event("message_stop", `{"type":"message_stop"}`)
	failure := event("error", `{"type":"error","error":{"type":"api_error","message":"Internal Server Error"}}`)
	good := []apitest.SSEEvent{start, block, delta, closeBlock, final, stop}
	replace := func(e apitest.SSEEvent, old, value string) apitest.SSEEvent {
		return event(e.Event, strings.ReplaceAll(string(e.Data), old, value))
	}
	tool := event("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"tool_use","id":"call_1","name":"weather","input":{}}}`)
	arg1 := event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"{\"city\":"}}`)
	arg2 := event("content_block_delta", `{"type":"content_block_delta","index":0,"delta":{"type":"input_json_delta","partial_json":"\"Paris\"}"}}`)
	toolFinal := replace(final, "end_turn", "tool_use")
	atOne := func(e apitest.SSEEvent) apitest.SSEEvent {
		return replace(e, `"index":0`, `"index":1`)
	}
	for _, tt := range []struct {
		name       string
		events     []apitest.SSEEvent
		allowError bool
		wantErr    bool
	}{
		{name: "text", events: good},
		{name: "fragmented-text", events: []apitest.SSEEvent{start, block, delta, delta, closeBlock, final, stop}},
		{name: "tools", events: []apitest.SSEEvent{start, tool, arg1, arg2, closeBlock, toolFinal, stop}},
		{name: "text-and-tools", events: []apitest.SSEEvent{start, block, delta, closeBlock, atOne(tool), atOne(arg1), atOne(arg2), atOne(closeBlock), toolFinal, stop}},
		{name: "duplicate-tool-id", events: []apitest.SSEEvent{start, tool, arg1, arg2, closeBlock, atOne(tool), atOne(arg1), atOne(arg2), atOne(closeBlock), toolFinal, stop}, wantErr: true},
		{name: "length", events: []apitest.SSEEvent{start, block, delta, closeBlock, replace(final, "end_turn", "max_tokens"), stop}},
		{name: "no-content", events: []apitest.SSEEvent{start, final, stop}},
		{name: "wrong-model", events: []apitest.SSEEvent{replace(start, `"model":"test"`, `"model":"other"`), final, stop}, wantErr: true},
		{name: "wrong-role", events: []apitest.SSEEvent{replace(start, "assistant", "user"), final, stop}, wantErr: true},
		{name: "negative-usage", events: []apitest.SSEEvent{start, replace(final, `"input_tokens":12`, `"input_tokens":-1`), stop}, wantErr: true},
		{name: "missing-usage", events: []apitest.SSEEvent{start, event("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`), stop}, wantErr: true},
		{name: "unknown-reason", events: []apitest.SSEEvent{start, replace(final, "end_turn", "mystery"), stop}, wantErr: true},
		{name: "tool-stop-without-tool", events: []apitest.SSEEvent{start, toolFinal, stop}, wantErr: true},
		{name: "wrong-delta-type", events: []apitest.SSEEvent{start, tool, delta, closeBlock, toolFinal, stop}, wantErr: true},
		{name: "invalid-tool-json", events: []apitest.SSEEvent{start, tool, arg1, closeBlock, toolFinal, stop}, wantErr: true},
		{name: "missing-tool-name", events: []apitest.SSEEvent{start, replace(tool, "weather", ""), arg1, arg2, closeBlock, toolFinal, stop}, wantErr: true},
		{name: "wrong-index", events: []apitest.SSEEvent{start, block, replace(delta, `"index":0`, `"index":1`), closeBlock, final, stop}, wantErr: true},
		{name: "duplicate-final", events: []apitest.SSEEvent{start, final, final, stop}, wantErr: true},
		{name: "duplicate-block-stop", events: []apitest.SSEEvent{start, block, closeBlock, closeBlock, final, stop}, wantErr: true},
		{name: "error-after-final", events: []apitest.SSEEvent{start, final, failure}, allowError: true, wantErr: true},
		{name: "malformed-error", events: []apitest.SSEEvent{event("error", `{"type":"error"}`)}, allowError: true, wantErr: true},
		{name: "error-before-start", events: []apitest.SSEEvent{failure}, allowError: true},
		{name: "error-open-block", events: []apitest.SSEEvent{start, block, failure}, allowError: true},
		{name: "empty", wantErr: true},
		{name: "truncated", events: good[:5], wantErr: true},
		{name: "missing-start", events: good[1:], wantErr: true},
		{name: "duplicate-start", events: []apitest.SSEEvent{start, start, final, stop}, wantErr: true},
		{name: "delta-before-block", events: []apitest.SSEEvent{start, delta, final, stop}, wantErr: true},
		{name: "overlapping-block", events: []apitest.SSEEvent{start, block, block, final, stop}, wantErr: true},
		{name: "unclosed-block", events: []apitest.SSEEvent{start, block, delta, final, stop}, wantErr: true},
		{name: "missing-final", events: []apitest.SSEEvent{start, stop}, wantErr: true},
		{name: "after-stop", events: append(append([]apitest.SSEEvent{}, good...), delta), wantErr: true},
		{name: "unexpected-error", events: []apitest.SSEEvent{failure}, wantErr: true},
		{name: "completed-after-error", events: []apitest.SSEEvent{start, failure, final, stop}, allowError: true, wantErr: true},
		{name: "name-mismatch", events: []apitest.SSEEvent{event("ping", string(start.Data))}, wantErr: true},
		{name: "malformed-json", events: []apitest.SSEEvent{event("message_start", `{`)}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, diff := validateMsgsStream(tt.events, "test", tt.allowError)
			if (diff != "") != tt.wantErr {
				t.Fatalf("validation: got %q, want error %v", diff, tt.wantErr)
			}
			if tt.name == "text" && (got.Text != "hello" || got.Usage.InputTokens != 12 || got.Usage.OutputTokens != 3 || got.StopReason != "end_turn") {
				t.Errorf("reconstruction: got %+v, want hello/end_turn/12/3", got)
			}
			if tt.name == "fragmented-text" && got.Text != "hellohello" {
				t.Errorf("text: got %q, want hellohello", got.Text)
			}
			if tt.name == "text-and-tools" && got.Text != "hello" {
				t.Errorf("text: got %q, want hello", got.Text)
			}
			if tt.name == "tools" || tt.name == "text-and-tools" {
				if len(got.Tools) != 1 {
					t.Fatalf("tools: got %d, want 1", len(got.Tools))
				}
				tool := got.Tools[0]
				input, ok := tool.Input.(map[string]any)
				if !ok || input["city"] != "Paris" || tool.ID != "call_1" || tool.Name != "weather" || got.StopReason != "tool_use" {
					t.Errorf("tool reconstruction: got %+v, want weather(call_1, Paris)/tool_use", got)
				}
			}
			if tt.name == "length" && got.StopReason != "max_tokens" {
				t.Errorf("stop reason: got %q, want max_tokens", got.StopReason)
			}
			if tt.allowError && !tt.wantErr && !got.Failed {
				t.Error("got completed stream, want failed stream")
			}
		})
	}
}
