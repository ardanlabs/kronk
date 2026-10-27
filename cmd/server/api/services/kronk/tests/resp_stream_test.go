package chatapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/cmd/server/app/sdk/apitest"
	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
)

// validateRespStream checks the wire contract, not the model's choice of words.
// Reasoning deltas currently have no item/content indexes. Tool argument deltas
// are complete snapshots, rather than arbitrary fragments (response.go).
func validateRespStream(events []apitest.SSEEvent) string {
	type itemState struct {
		item     kronk.ResponseOutputItem
		text     string
		args     string
		part     bool
		textDone bool
		partDone bool
		argsDone bool
		done     bool
	}
	items := map[int]*itemState{}
	ids := map[string]bool{}
	var start *kronk.ResponseResponse
	terminal := false
	for i, wire := range events {
		var e kronk.ResponseStreamEvent
		if err := json.Unmarshal(wire.Data, &e); err != nil {
			return fmt.Sprintf("event %d: decoding: %s", i, err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(wire.Data, &fields); err != nil {
			return fmt.Sprintf("event %d: fields: %s", i, err)
		}
		if terminal || e.Type == "" || wire.Event != e.Type || fields["sequence_number"] == nil || string(fields["sequence_number"]) == "null" || e.SequenceNumber != i {
			return fmt.Sprintf("event %d: invalid name, sequence, or event after termination", i)
		}
		// An explicit error terminates the stream without item.done/completed.
		if e.Type == "error" {
			if e.Message == "" {
				return "error event missing message"
			}
			terminal = true
			continue
		}
		if i < 2 {
			want := "response.created"
			if i == 1 {
				want = "response.in_progress"
			}
			if e.Type != want || e.Response == nil || e.Response.Status != "in_progress" || e.Response.Object != "response" || !strings.HasPrefix(e.Response.ID, "resp_") || e.Response.CreatedAt <= 0 || e.Response.Model == "" || len(e.Response.Output) != 0 {
				return fmt.Sprintf("event %d: invalid start", i)
			}
			if start != nil && (start.ID != e.Response.ID || start.CreatedAt != e.Response.CreatedAt || start.Model != e.Response.Model) {
				return "start response identity changed"
			}
			start = e.Response
			continue
		}
		if e.Type == "response.completed" || e.Type == "response.incomplete" || e.Type == "response.failed" {
			r := e.Response
			if r == nil || r.ID != start.ID || r.CreatedAt != start.CreatedAt || r.Model != start.Model || r.Object != "response" || e.Type != "response."+r.Status {
				return "invalid final response identity/status"
			}
			if r.Status == "completed" && (r.CompletedAt == nil || *r.CompletedAt < r.CreatedAt || r.Error != nil || r.IncompleteDetail != nil) {
				return "invalid completed metadata"
			}
			if r.Status == "incomplete" && (r.IncompleteDetail == nil || r.IncompleteDetail.Reason != "max_output_tokens" || r.CompletedAt != nil) {
				return "invalid incomplete metadata"
			}
			if r.Status == "failed" && (r.Error == nil || r.Error.Message == "" || r.CompletedAt != nil) {
				return "invalid failed metadata"
			}
			u := r.Usage
			if u.InputTokens < 0 || u.OutputTokens < 0 || u.TotalTokens != u.InputTokens+u.OutputTokens || u.InputTokensDetails.CachedTokens < 0 || u.InputTokensDetails.CachedTokens > u.InputTokens || u.OutputTokenDetail.ReasoningTokens < 0 || u.OutputTokenDetail.ReasoningTokens > u.OutputTokens {
				return "invalid usage accounting"
			}
			if len(r.Output) != len(items) {
				return "final output count changed"
			}
			for j, out := range r.Output {
				s := items[j]
				if s == nil || !s.done {
					return "final response before item done"
				}
				want := s.item
				want.Status = r.Status // item.done is completed even for length/error finishes.
				if diff := cmp.Diff(want, out); diff != "" {
					return "final output differs from item.done: " + diff
				}
			}
			terminal = true
			continue
		}
		if e.Type == "response.reasoning_summary_text.delta" {
			continue
		}
		if e.OutputIndex == nil || *e.OutputIndex < 0 {
			return "missing output index"
		}
		idx := *e.OutputIndex
		if e.Type == "response.output_item.added" {
			if e.Item == nil || e.Item.ID == "" || ids[e.Item.ID] || items[idx] != nil || idx != len(items) || e.Item.Status != "in_progress" {
				return "invalid item added"
			}
			switch e.Item.Type {
			case "message":
				if !strings.HasPrefix(e.Item.ID, "msg_") || e.Item.Role != "assistant" || len(e.Item.Content) != 1 || e.Item.Content[0].Type != "output_text" || e.Item.Content[0].Text != "" {
					return "invalid message item"
				}
			case "function_call":
				if !strings.HasPrefix(e.Item.ID, "call_") || e.Item.CallID == "" || e.Item.Name == "" || e.Item.Arguments == nil || *e.Item.Arguments != "" {
					return "invalid function item"
				}
			default:
				return "unsupported item type"
			}
			items[idx] = &itemState{item: *e.Item}
			ids[e.Item.ID] = true
			continue
		}
		s := items[idx]
		if s == nil || s.done {
			return "event before item added or after item done"
		}
		if e.Type == "response.output_item.done" {
			if e.Item == nil || e.Item.ID != s.item.ID || e.Item.Status != "completed" {
				return "invalid item done"
			}
			want := s.item
			want.Status = "completed"
			if want.Type == "message" {
				if !s.partDone {
					return "message done before content done"
				}
				want.Content = []kronk.ResponseContentItem{{Type: "output_text", Text: s.text, Annotations: []string{}}}
			} else {
				if !s.argsDone {
					return "tool done before arguments done"
				}
				want.Arguments = &s.args
			}
			if diff := cmp.Diff(want, *e.Item); diff != "" {
				return "item done differs: " + diff
			}
			s.item = *e.Item
			s.done = true
			continue
		}
		if e.ItemID != s.item.ID {
			return "item ID changed"
		}
		if strings.HasPrefix(e.Type, "response.function_call_arguments.") {
			if s.item.Type != "function_call" || s.argsDone {
				return "invalid tool argument ordering"
			}
			switch e.Type {
			case "response.function_call_arguments.delta":
				s.args = e.Delta
			case "response.function_call_arguments.done":
				if e.Name != s.item.Name || e.Arguments != s.args || !json.Valid([]byte(e.Arguments)) {
					return "tool arguments not reconstructed"
				}
				s.argsDone = true
			default:
				return "unknown tool event"
			}
			continue
		}
		if s.item.Type != "message" || e.ContentIndex == nil || *e.ContentIndex != 0 {
			return "invalid content index/type"
		}
		switch e.Type {
		case "response.content_part.added":
			if s.part || e.Part == nil || e.Part.Type != "output_text" || e.Part.Text != "" {
				return "invalid content part added"
			}
			s.part = true
		case "response.output_text.delta":
			if !s.part || s.textDone {
				return "text delta out of order"
			}
			s.text += e.Delta
		case "response.output_text.done":
			if !s.part || s.textDone || e.Text != s.text {
				return "text not reconstructed"
			}
			s.textDone = true
		case "response.content_part.done":
			if !s.textDone || s.partDone || e.Part == nil || e.Part.Type != "output_text" || e.Part.Text != s.text {
				return "invalid content part done"
			}
			s.partDone = true
		default:
			return "unknown response event: " + e.Type
		}
	}
	if !terminal {
		return "stream missing terminal event"
	}
	return ""
}

// TestResponsesStreamValidation runs without loading a model or native libraries.
func TestResponsesStreamValidation(t *testing.T) {
	for _, kind := range []string{"text", "tool", "partial-error"} {
		t.Run(kind+"-multiple-deltas", func(t *testing.T) {
			fixtureKind, deltaIndex := "text", 4
			if kind == "tool" {
				fixtureKind, deltaIndex = "tool", 3
			}
			wire := respStreamFixture(t, fixtureKind)
			var events []kronk.ResponseStreamEvent
			for _, w := range wire {
				var e kronk.ResponseStreamEvent
				if err := json.Unmarshal(w.Data, &e); err != nil {
					t.Fatal(err)
				}
				events = append(events, e)
			}
			if kind == "partial-error" {
				events = append(events[:deltaIndex+1], kronk.ResponseStreamEvent{Type: "error", Message: "cancelled"})
			} else {
				delta := events[deltaIndex]
				if kind == "text" {
					events[deltaIndex].Delta = "H"
					delta.Delta = "i"
				} else {
					events[deltaIndex].Delta = `{}`
				}
				tail := append([]kronk.ResponseStreamEvent{delta}, events[deltaIndex+1:]...)
				events = append(events[:deltaIndex+1], tail...)
			}
			wire = nil
			for i, e := range events {
				e.SequenceNumber = i
				data, err := json.Marshal(e)
				if err != nil {
					t.Fatal(err)
				}
				wire = append(wire, apitest.SSEEvent{Event: e.Type, Data: data})
			}
			if diff := validateRespStream(wire); diff != "" {
				t.Fatal(diff)
			}
		})
	}
	for _, kind := range []string{"text", "tool", "length", "failed", "error"} {
		t.Run(kind, func(t *testing.T) {
			events := respStreamFixture(t, kind)
			if diff := validateRespStream(events); diff != "" {
				t.Fatalf("valid stream: %s", diff)
			}
		})
	}
	for _, tt := range []struct {
		name  string
		kind  string
		index int
		field string
		value any
	}{
		{"named-event", "text", 4, "type", "response.output_text.done"},
		{"sequence-gap", "text", 4, "sequence_number", 99},
		{"missing-sequence", "text", 0, "sequence_number", nil},
		{"wrong-start", "text", 0, "type", "response.in_progress"},
		{"item-id", "text", 4, "item_id", "msg_other"},
		{"output-index", "text", 4, "output_index", 8},
		{"content-index", "text", 4, "content_index", 1},
		{"text-reconstruction", "text", 5, "text", "wrong"},
		{"arguments-reconstruction", "tool", 4, "arguments", "{}"},
		{"tool-name", "tool", 4, "name", "wrong"},
		{"empty-error", "error", 2, "message", ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := respStreamFixture(t, tt.kind)
			var data map[string]any
			if err := json.Unmarshal(events[tt.index].Data, &data); err != nil {
				t.Fatal(err)
			}
			data[tt.field] = tt.value
			encoded, err := json.Marshal(data)
			if err != nil {
				t.Fatal(err)
			}
			events[tt.index].Data = encoded
			if diff := validateRespStream(events); diff == "" {
				t.Fatal("invalid stream accepted")
			}
		})
	}
	for _, kind := range []string{"text", "tool", "length", "failed", "error"} {
		t.Run(kind+"-truncated", func(t *testing.T) {
			events := respStreamFixture(t, kind)
			if validateRespStream(events[:len(events)-1]) == "" {
				t.Fatal("truncated stream accepted")
			}
		})
		t.Run(kind+"-after-terminal", func(t *testing.T) {
			events := respStreamFixture(t, kind)
			events = append(events, events[len(events)-1])
			if validateRespStream(events) == "" {
				t.Fatal("event after terminal accepted")
			}
		})
	}
	for _, tt := range []struct {
		name   string
		mutate func(*kronk.ResponseStreamEvent)
	}{
		{"final-id", func(e *kronk.ResponseStreamEvent) { e.Response.ID = "resp_changed" }},
		{"final-text", func(e *kronk.ResponseStreamEvent) { e.Response.Output[0].Content[0].Text = "changed" }},
		{"final-status", func(e *kronk.ResponseStreamEvent) { e.Response.Status = "incomplete" }},
		{"missing-completed-at", func(e *kronk.ResponseStreamEvent) { e.Response.CompletedAt = nil }},
		{"usage-total", func(e *kronk.ResponseStreamEvent) { e.Response.Usage.TotalTokens++ }},
		{"usage-cached", func(e *kronk.ResponseStreamEvent) { e.Response.Usage.InputTokensDetails.CachedTokens = 99 }},
		{"usage-reasoning", func(e *kronk.ResponseStreamEvent) { e.Response.Usage.OutputTokenDetail.ReasoningTokens = 99 }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			events := respStreamFixture(t, "text")
			last := len(events) - 1
			var event kronk.ResponseStreamEvent
			if err := json.Unmarshal(events[last].Data, &event); err != nil {
				t.Fatal(err)
			}
			tt.mutate(&event)
			data, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			events[last].Data = data
			if validateRespStream(events) == "" {
				t.Fatal("invalid final response accepted")
			}
		})
	}
	// Removing an event and renumbering must still fail; sequence checks alone
	// cannot prove the lifecycle was complete.
	for _, index := range []int{0, 1, 2, 3, 5, 6, 7} {
		t.Run(fmt.Sprintf("missing-lifecycle-%d", index), func(t *testing.T) {
			events := respStreamFixture(t, "text")
			events = append(events[:index], events[index+1:]...)
			for i := range events {
				var e kronk.ResponseStreamEvent
				if err := json.Unmarshal(events[i].Data, &e); err != nil {
					t.Fatal(err)
				}
				e.SequenceNumber = i
				data, err := json.Marshal(e)
				if err != nil {
					t.Fatal(err)
				}
				events[i].Data = data
			}
			if validateRespStream(events) == "" {
				t.Fatal("missing lifecycle event accepted")
			}
		})
	}
}

func respStreamFixture(t *testing.T, kind string) []apitest.SSEEvent {
	t.Helper()
	start := kronk.ResponseResponse{ID: "resp_test", Object: "response", CreatedAt: 1, Status: "in_progress", Model: qwen3ModelID}
	events := []kronk.ResponseStreamEvent{
		{Type: "response.created", Response: &start},
		{Type: "response.in_progress", Response: &start},
	}
	final := start
	final.Status = "completed"
	final.CompletedAt = new(int64(2))
	final.Usage = kronk.ResponseUsage{InputTokens: 3, OutputTokens: 2, TotalTokens: 5}
	if kind == "error" {
		events = append(events, kronk.ResponseStreamEvent{Type: "error", Message: "inference failed"})
	} else {
		var done kronk.ResponseOutputItem
		if kind == "tool" {
			added := kronk.ResponseOutputItem{Type: "function_call", ID: "call_test", CallID: "tool_test", Name: "weather", Status: "in_progress", Arguments: new("")}
			done = added
			done.Status = "completed"
			done.Arguments = new(`{"city":"Boston"}`)
			events = append(events,
				kronk.ResponseStreamEvent{Type: "response.output_item.added", OutputIndex: new(0), Item: &added},
				kronk.ResponseStreamEvent{Type: "response.function_call_arguments.delta", OutputIndex: new(0), ItemID: added.ID, Delta: *done.Arguments},
				kronk.ResponseStreamEvent{Type: "response.function_call_arguments.done", OutputIndex: new(0), ItemID: added.ID, Name: added.Name, Arguments: *done.Arguments},
			)
		} else {
			empty := kronk.ResponseContentItem{Type: "output_text", Annotations: []string{}}
			part := empty
			part.Text = "Hi"
			added := kronk.ResponseOutputItem{Type: "message", ID: "msg_test", Status: "in_progress", Role: "assistant", Content: []kronk.ResponseContentItem{empty}}
			done = added
			done.Status = "completed"
			done.Content = []kronk.ResponseContentItem{part}
			events = append(events,
				kronk.ResponseStreamEvent{Type: "response.output_item.added", OutputIndex: new(0), Item: &added},
				kronk.ResponseStreamEvent{Type: "response.content_part.added", OutputIndex: new(0), ContentIndex: new(0), ItemID: added.ID, Part: &empty},
				kronk.ResponseStreamEvent{Type: "response.output_text.delta", OutputIndex: new(0), ContentIndex: new(0), ItemID: added.ID, Delta: "Hi"},
				kronk.ResponseStreamEvent{Type: "response.output_text.done", OutputIndex: new(0), ContentIndex: new(0), ItemID: added.ID, Text: "Hi"},
				kronk.ResponseStreamEvent{Type: "response.content_part.done", OutputIndex: new(0), ContentIndex: new(0), ItemID: added.ID, Part: &part},
			)
		}
		events = append(events, kronk.ResponseStreamEvent{Type: "response.output_item.done", OutputIndex: new(0), Item: &done})
		out := done
		switch kind {
		case "length":
			final.Status = "incomplete"
			final.CompletedAt = nil
			final.IncompleteDetail = &kronk.IncompleteDetail{Reason: "max_output_tokens"}
		case "failed":
			final.Status = "failed"
			final.CompletedAt = nil
			final.Error = &kronk.ResponseError{Code: "error", Message: "inference failed"}
		}
		out.Status = final.Status
		final.Output = []kronk.ResponseOutputItem{out}
		events = append(events, kronk.ResponseStreamEvent{Type: "response." + final.Status, Response: &final})
	}
	wire := make([]apitest.SSEEvent, len(events))
	for i, e := range events {
		e.SequenceNumber = i
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		wire[i] = apitest.SSEEvent{Event: e.Type, Data: data}
	}
	return wire
}

// respStreamContractsQwen3 adds deterministic length and forced-tool contracts.
// Runtime failures are exercised synthetically: provoking native inference
// errors via an otherwise valid HTTP request is not a stable API contract.
func respStreamContractsQwen3(t *testing.T, tokens map[string]string) []apitest.Table {
	t.Helper()
	var table []apitest.Table
	for _, kind := range []string{"length", "tool"} {
		input := model.D{
			"model": qwen3ModelID, "input": "Count from one to one hundred.",
			"stream": true, "max_output_tokens": 1, "temperature": 0,
			"chat_template_kwargs": model.D{"enable_thinking": false},
			"stream_options":       model.D{"include_usage": true},
		}
		if kind == "tool" {
			input["input"] = "Call weather for Boston."
			input["max_output_tokens"] = 512
			input["tools"] = []model.D{{"type": "function", "name": "weather", "description": "Get weather for a city", "parameters": model.D{"type": "object", "properties": model.D{"city": model.D{"type": "string"}}, "required": []string{"city"}}}}
			input["tool_choice"] = model.D{"type": "function", "name": "weather"}
		}
		table = append(table, apitest.Table{
			Name: "stream-" + kind, URL: "/v1/responses", Token: tokens["responses"], Method: http.MethodPost, StatusCode: http.StatusOK,
			Input: input, GotResp: &kronk.ResponseStreamEvent{},
			SSECmpFunc: validateRespStream,
			CmpFunc: func(got any, _ any) string {
				e := got.(*kronk.ResponseStreamEvent)
				if e.Response == nil {
					return "expected final response"
				}
				r := e.Response
				if r.Usage.InputTokens <= 0 || r.Usage.OutputTokens <= 0 {
					return "expected nonzero usage"
				}
				if kind == "length" {
					if e.Type != "response.incomplete" || r.IncompleteDetail == nil || r.IncompleteDetail.Reason != "max_output_tokens" || r.Usage.OutputTokens > 1 {
						return "expected length-limited incomplete response"
					}
					return ""
				}
				if e.Type != "response.completed" {
					return "expected completed tool response"
				}
				for _, item := range r.Output {
					if item.Type != "function_call" {
						continue
					}
					if item.Name != "weather" || item.Arguments == nil {
						return "expected weather call"
					}
					var args map[string]any
					if err := json.Unmarshal([]byte(*item.Arguments), &args); err != nil {
						return "tool arguments must be a JSON object: " + err.Error()
					}
					if city, ok := args["city"].(string); !ok || !containsIgnoreCase(city, "Boston") {
						return "expected city argument naming Boston"
					}
					return ""
				}
				return "missing function call"
			},
		})
	}
	return table
}
