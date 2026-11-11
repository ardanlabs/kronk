package kronk

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/google/go-cmp/cmp"
)

func TestStreamStateErrorEvent(t *testing.T) {
	ss := streamState{}
	ss.start()

	event := ss.errorEvent(errors.New("inference failed"))

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := map[string]any{
		"type":            "error",
		"sequence_number": float64(2),
		"message":         "inference failed",
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("error event mismatch (-want +got):\n%s", diff)
	}
}

func TestStreamStateOutputIndexesRemainStable(t *testing.T) {
	ss := streamState{}
	messageEvents := ss.handleTextDelta("hello")

	firstToolEvents := ss.handleToolCalls([]model.ResponseToolCall{{
		ID:       "tool-1",
		Function: model.ResponseToolCallFunction{Name: "first"},
	}})
	secondToolEvents := ss.handleToolCalls([]model.ResponseToolCall{{
		ID:       "tool-2",
		Function: model.ResponseToolCallFunction{Name: "second"},
	}})
	doneEvents := ss.finalizeToolCalls()

	assertEventIndexes(t, messageEvents, 0)
	assertEventIndexes(t, firstToolEvents, 1)
	assertEventIndexes(t, secondToolEvents, 2)
	assertEventIndexes(t, doneEvents[:2], 1)
	assertEventIndexes(t, doneEvents[2:], 2)
}

func TestStreamStateIndexesMessageAfterTool(t *testing.T) {
	ss := streamState{}
	toolEvents := ss.handleToolCalls([]model.ResponseToolCall{{
		ID:       "tool-1",
		Function: model.ResponseToolCallFunction{Name: "first"},
	}})
	messageEvents := ss.handleTextDelta("hello")

	assertEventIndexes(t, toolEvents, 0)
	assertEventIndexes(t, messageEvents, 1)
}

func assertEventIndexes(t *testing.T, events []ResponseStreamEvent, want int) {
	t.Helper()

	for _, event := range events {
		if event.OutputIndex == nil {
			t.Fatalf("%s: OutputIndex is nil", event.Type)
		}
		if got := *event.OutputIndex; got != want {
			t.Errorf("%s: OutputIndex got %d, want %d", event.Type, got, want)
		}
	}
}

func TestResponseValidatesMessagesBeforeAdmission(t *testing.T) {
	tests := []struct {
		name string
		d    model.D
		want error
	}{
		{name: "missing", d: model.D{}, want: model.ErrMessagesMissing},
		{name: "invalid", d: model.D{"messages": "invalid"}, want: model.ErrMessagesInvalid},
		{name: "empty input", d: model.D{"input": []any{}}, want: model.ErrMessagesMissing},
		{name: "nil input", d: model.D{"input": nil}, want: model.ErrMessagesInvalid},
		{name: "scalar input", d: model.D{"input": 42}, want: model.ErrMessagesInvalid},
		{name: "required without tools", d: model.D{"input": "hello", "tool_choice": "required"}, want: model.ErrInvalidRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var krn Kronk

			if _, err := krn.Response(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("Response: got %v, want %v", err, tt.want)
			}
			if _, err := krn.ResponseStreaming(t.Context(), tt.d); !errors.Is(err, tt.want) {
				t.Errorf("ResponseStreaming: got %v, want %v", err, tt.want)
			}
		})
	}
}

func TestConvertInputToMessagesPreservesStop(t *testing.T) {
	for _, value := range []any{"END", []any{"DONE", "STOP"}} {
		d := model.D{"input": "hello", "stop": value}

		got, err := convertInputToMessages(d)
		if err != nil {
			t.Fatalf("convertInputToMessages stop %v: %v", value, err)
		}
		if err := model.ValidateChatRequest(got); err != nil {
			t.Fatalf("ValidateChatRequest stop %v: %v", value, err)
		}
		if diff := cmp.Diff(value, got["stop"]); diff != "" {
			t.Errorf("stop mismatch (-want +got):\n%s", diff)
		}
	}
}

func TestToChatResponseToResponsesUsageIncludesReasoning(t *testing.T) {
	chatResp := model.ChatResponse{
		Usage: &model.Usage{
			PromptTokens:        100,
			PromptTokensDetails: model.PromptTokensDetails{CachedTokens: 80},
			CompletionTokens:    25,
			CompletionTokensDetails: model.CompletionTokensDetails{
				ReasoningTokens: 20,
			},
			TotalTokens: 125,
		},
	}

	resp := toChatResponseToResponses(chatResp, model.D{})

	if got, want := resp.Usage.OutputTokens, 25; got != want {
		t.Errorf("OutputTokens: got %d, want %d", got, want)
	}
	if got, want := resp.Usage.InputTokensDetails.CachedTokens, 80; got != want {
		t.Errorf("CachedTokens: got %d, want %d", got, want)
	}
	if got, want := resp.Usage.OutputTokenDetail.ReasoningTokens, 20; got != want {
		t.Errorf("ReasoningTokens: got %d, want %d", got, want)
	}
	if got, want := resp.Usage.TotalTokens, 125; got != want {
		t.Errorf("TotalTokens: got %d, want %d", got, want)
	}
}

func TestToChatResponseToResponsesWithoutUsage(t *testing.T) {
	resp := toChatResponseToResponses(model.ChatResponse{}, model.D{})

	if got := resp.Usage; got != (ResponseUsage{}) {
		t.Errorf("Usage: got %+v, want zero value", got)
	}
}

func TestToChatResponseToResponsesPreservesCreatedTimestamp(t *testing.T) {
	const created = int64(1_787_077_112)

	resp := toChatResponseToResponses(model.ChatResponse{Created: created}, model.D{})

	if resp.CreatedAt != created {
		t.Errorf("CreatedAt: got %d, want %d", resp.CreatedAt, created)
	}
}

func TestToChatResponseToResponsesPreservesToolChoice(t *testing.T) {
	toolChoice := model.D{"type": "function", "function": model.D{"name": "get_weather"}}
	want := model.D{"type": "function", "name": "get_weather"}
	chatResp := model.ChatResponse{Usage: &model.Usage{}}

	resp := toChatResponseToResponses(chatResp, model.D{"tool_choice": toolChoice})

	if diff := cmp.Diff(any(want), resp.ToolChoice); diff != "" {
		t.Errorf("tool choice mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertInputToMessagesAcceptsResponsesToolChoice(t *testing.T) {
	toolChoice := model.D{"type": "function", "name": "get_weather"}
	d := model.D{
		"input":       "hello",
		"tool_choice": toolChoice,
		"tools": []model.D{
			{"type": "function", "name": "get_weather"},
		},
	}

	got, err := convertInputToMessages(d)
	if err != nil {
		t.Fatalf("convertInputToMessages: %v", err)
	}
	if err := model.ValidateChatRequest(got); err != nil {
		t.Fatalf("ValidateChatRequest: %v", err)
	}
	want := model.D{"type": "function", "function": model.D{"name": "get_weather"}}
	if diff := cmp.Diff(any(want), got["tool_choice"]); diff != "" {
		t.Errorf("tool choice mismatch (-want +got):\n%s", diff)
	}
	wantTools := []model.D{{
		"type":     "function",
		"function": model.D{"name": "get_weather"},
	}}
	if diff := cmp.Diff(wantTools, got["tools"]); diff != "" {
		t.Errorf("tools mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertInputToMessagesRejectsChatToolChoice(t *testing.T) {
	d := model.D{
		"input": "hello",
		"tool_choice": model.D{
			"type":     "function",
			"function": model.D{"name": "get_weather"},
		},
	}

	_, err := convertInputToMessages(d)
	if !errors.Is(err, model.ErrInvalidRequest) {
		t.Fatalf("convertInputToMessages: got %v, want ErrInvalidRequest", err)
	}
}

func TestToChatResponseToResponsesTokenLimit(t *testing.T) {
	finishReason := model.FinishReasonLength
	chatResp := model.ChatResponse{
		Choices: []model.Choice{{
			Message:         &model.ResponseMessage{Content: "partial"},
			FinishReasonPtr: &finishReason,
		}},
		Usage: &model.Usage{},
	}

	resp := toChatResponseToResponses(chatResp, model.D{})
	if got, want := resp.Status, "incomplete"; got != want {
		t.Errorf("Status: got %q, want %q", got, want)
	}
	if resp.CompletedAt != nil {
		t.Errorf("CompletedAt: got %d, want nil", *resp.CompletedAt)
	}
	if resp.IncompleteDetail == nil {
		t.Fatal("IncompleteDetail: got nil, want max_output_tokens")
	}
	if got, want := resp.IncompleteDetail.Reason, "max_output_tokens"; got != want {
		t.Errorf("IncompleteDetail.Reason: got %q, want %q", got, want)
	}
}

func TestStreamStateCompleteTokenLimit(t *testing.T) {
	finishReason := model.FinishReasonLength
	chatResp := model.ChatResponse{
		Choices: []model.Choice{{FinishReasonPtr: &finishReason}},
		Usage: &model.Usage{
			PromptTokensDetails: model.PromptTokensDetails{CachedTokens: 42},
		},
	}
	ss := streamState{}

	events := ss.complete(chatResp)
	if len(events) == 0 {
		t.Fatal("complete: got no events")
	}
	last := events[len(events)-1]
	if got, want := last.Type, "response.incomplete"; got != want {
		t.Errorf("event type: got %q, want %q", got, want)
	}
	if last.Response == nil {
		t.Fatal("event response: got nil")
	}
	if got, want := last.Response.Status, "incomplete"; got != want {
		t.Errorf("response status: got %q, want %q", got, want)
	}
	if got, want := last.Response.Usage.InputTokensDetails.CachedTokens, 42; got != want {
		t.Errorf("CachedTokens: got %d, want %d", got, want)
	}
}

func TestConvertInputToMessagesRoleShapedImage(t *testing.T) {
	const imageURL = "data:image/jpeg;base64,aW1hZ2U="

	input := model.D{
		"input": []model.D{
			{
				"role": "user",
				"content": []model.D{
					{"type": "input_image", "image_url": imageURL},
					{"type": "input_text", "text": "describe this image"},
				},
			},
		},
	}

	want := model.D{
		"messages": []model.D{
			{
				"role": "user",
				"content": []model.D{
					{
						"type": "image_url",
						"image_url": model.D{
							"url": imageURL,
						},
					},
					{"type": "text", "text": "describe this image"},
				},
			},
		},
	}

	got, err := convertInputToMessages(input)
	if err != nil {
		t.Fatalf("convertInputToMessages: %v", err)
	}
	if diff := cmp.Diff(want, got); diff != "" {
		t.Errorf("convertInputToMessages mismatch (-want +got):\n%s", diff)
	}
}

func TestConvertInputToMessagesMapsMaxOutputTokens(t *testing.T) {
	d := model.D{
		"input":             "hello",
		"max_output_tokens": float64(32),
		"max_tokens":        16,
	}

	got, err := convertInputToMessages(d)
	if err != nil {
		t.Fatalf("convertInputToMessages: %v", err)
	}

	if got["max_tokens"] != float64(32) {
		t.Errorf("max_tokens: got %v, want 32", got["max_tokens"])
	}
	params := extractInputParams(got)
	if params.MaxOutputTokens == nil || *params.MaxOutputTokens != 32 {
		t.Errorf("MaxOutputTokens: got %v, want 32", params.MaxOutputTokens)
	}
}

func TestConvertInputToMessagesRejectsFileInput(t *testing.T) {
	tests := []struct {
		name  string
		input model.D
	}{
		{
			name: "role-shaped input",
			input: model.D{
				"input": []model.D{
					{
						"role": "user",
						"content": []model.D{
							{"type": "input_file", "filename": "document.pdf", "file_data": "data:application/pdf;base64,JVBERg=="},
						},
					},
				},
			},
		},
		{
			name: "flat input",
			input: model.D{
				"input": []model.D{
					{"type": "input_file", "filename": "document.txt", "file_data": "dGV4dA=="},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := convertInputToMessages(tt.input)
			if err == nil {
				t.Fatal("convertInputToMessages: expected error")
			}
			if !errors.Is(err, model.ErrFileInputsUnsupported) {
				t.Errorf("error: got %v, want ErrFileInputsUnsupported", err)
			}
			if got, want := err.Error(), "convert-input-to-messages: file inputs are not currently supported"; got != want {
				t.Errorf("error: got %q, want %q", got, want)
			}
		})
	}
}

func TestResponsesNativeControls(t *testing.T) {
	schema := model.D{"type": "integer", "enum": []any{json.Number("9007199254740993")}}
	format := model.D{"type": "json_schema", "name": "answer", "schema": schema, "strict": false}
	d := model.D{
		"input": "hello", "text": model.D{"format": format},
		"reasoning": model.D{"effort": "low", "summary": "auto"},
	}
	got, err := convertInputToMessages(d)
	if err != nil {
		t.Fatal(err)
	}
	want := model.D{"type": "json_schema", "json_schema": model.D{"name": "answer", "schema": schema, "strict": false}}
	if diff := cmp.Diff(any(want), got["response_format"]); diff != "" {
		t.Errorf("response_format (-want +got):\n%s", diff)
	}
	if d["reasoning_effort"] != "low" {
		t.Errorf("in-place reasoning effort: got %v, want low", d["reasoning_effort"])
	}
	if format["type"] != "json_schema" {
		t.Fatal("normalization changed the caller's nested format")
	}
	for _, typ := range []string{"text", "json_object"} {
		legacy := model.D{"type": "json_object"}
		d := model.D{
			"messages": []model.D{{"role": "user", "content": "existing"}}, "input": "ignored",
			"text": model.D{"format": model.D{"type": typ}}, "response_format": legacy,
			"reasoning": model.D{"effort": "low"}, "reasoning_effort": "high",
		}
		got, err := convertInputToMessages(d)
		if err != nil {
			t.Fatal(err)
		}
		if diff := cmp.Diff(any(legacy), got["response_format"]); diff != "" {
			t.Errorf("legacy format precedence (-want +got):\n%s", diff)
		}
		if got["reasoning_effort"] != "high" || got["messages"].([]model.D)[0]["content"] != "existing" {
			t.Fatalf("legacy controls/messages were overridden: %v", got)
		}
	}
}

func TestResponsesFormatPrecedence(t *testing.T) {
	native := model.D{"format": model.D{"type": "json_schema", "name": "native", "schema": model.D{"type": "boolean"}}}
	legacy := model.D{"type": "json_schema", "json_schema": model.D{"name": "legacy", "schema": model.D{"type": "integer"}}}
	for _, tt := range []struct {
		name   string
		fields model.D
		want   ResponseFormatType
	}{
		{"native text", model.D{"text": model.D{"format": model.D{"type": "text"}}}, ResponseFormatType{Type: "text"}},
		{"native object", model.D{"text": model.D{"format": model.D{"type": "json_object"}}}, ResponseFormatType{Type: "json_object"}},
		{"native schema", model.D{"text": native}, ResponseFormatType{Type: "json_schema", Name: "native", Schema: new(model.D{"type": "boolean"})}},
		{"legacy over native", model.D{"text": native, "response_format": legacy}, ResponseFormatType{Type: "json_schema", Name: "legacy", Schema: new(model.D{"type": "integer"})}},
		{"grammar over formats", model.D{"text": native, "response_format": legacy, "grammar": `root ::= "custom"`}, ResponseFormatType{Type: "text"}},
		{"schema over grammar and formats", model.D{"text": native, "response_format": legacy, "grammar": `root ::= "custom"`, "json_schema": model.D{"type": "string"}}, ResponseFormatType{Type: "json_schema", Name: "response", Schema: new(model.D{"type": "string"})}},
		{"empty grammar allows native", model.D{"text": native, "grammar": ""}, ResponseFormatType{Type: "json_schema", Name: "native", Schema: new(model.D{"type": "boolean"})}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.fields.ShallowClone()
			d["input"] = "hello"
			request := d.ShallowClone()
			normalized, err := convertInputToMessages(d)
			if err != nil {
				t.Fatal(err)
			}
			// Check both the original request and the normalized document: metadata
			// must agree even after native controls become chat controls.
			for _, document := range []model.D{request, normalized} {
				resp := toChatResponseToResponses(model.ChatResponse{}, document)
				if diff := cmp.Diff(tt.want, resp.Text.Format); diff != "" {
					t.Errorf("effective format (-want +got):\n%s", diff)
				}
			}
			if tt.name == "native text" || tt.name == "native object" {
				if diff := cmp.Diff(any(tt.fields["text"].(model.D)["format"]), normalized["response_format"]); diff != "" {
					t.Errorf("standalone native mapping (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func TestResponsesNonStreamingReasoningOutput(t *testing.T) {
	for _, finish := range []string{model.FinishReasonStop, model.FinishReasonLength} {
		for _, reasoning := range []string{"consider the evidence", ""} {
			t.Run(finish+"/"+reasoning, func(t *testing.T) {
				resp := toChatResponseToResponses(model.ChatResponse{Choices: []model.Choice{{
					Message: &model.ResponseMessage{Content: "answer", Reasoning: reasoning}, FinishReasonPtr: &finish,
				}}}, model.D{"reasoning": model.D{"effort": "low", "summary": "detailed"}})
				status := "completed"
				if finish == model.FinishReasonLength {
					status = "incomplete"
				}
				if resp.Status != status || resp.Reasoning.Summary == nil || *resp.Reasoning.Summary != "detailed" || resp.Reasoning.Effort == nil || *resp.Reasoning.Effort != "low" {
					t.Fatalf("status or reasoning configuration: got %+v", resp)
				}
				items := resp.Output
				if reasoning != "" {
					if len(items) != 2 || items[0].Type != "reasoning" || !strings.HasPrefix(items[0].ID, "rs_") || items[0].Status != status {
						t.Fatalf("reasoning item: got %+v", items)
					}
					if diff := cmp.Diff([]ResponseSummaryPart{{Type: "summary_text", Text: reasoning}}, items[0].Summary); diff != "" {
						t.Errorf("generated reasoning (-want +got):\n%s", diff)
					}
					items = items[1:]
				}
				if len(items) != 1 || items[0].Type != "message" || items[0].Status != status || len(items[0].Content) != 1 || items[0].Content[0].Text != "answer" {
					t.Errorf("answer item: got %+v", items)
				}
			})
		}
	}
}

func TestResponsesRejectMalformedNativeControls(t *testing.T) {
	for _, fields := range []model.D{
		{"text": "invalid"},
		{"text": model.D{"format": []any{}}},
		{"text": model.D{"format": model.D{"type": "xml"}}},
		{"text": model.D{"format": model.D{"type": "json_schema", "name": "answer"}}},
		{"text": model.D{"format": model.D{"type": "json_schema", "schema": model.D{}}}},
		{"text": model.D{"format": model.D{"type": "json_schema", "name": "answer", "schema": model.D{}, "strict": "true"}}},
		{"text": model.D{"format": model.D{"type": "json_schema", "name": "answer", "schema": model.D{}, "description": false}}},
		{"reasoning": "invalid"},
		{"reasoning": model.D{"effort": 5}},
		{"reasoning": model.D{"summary": "generated reasoning is not a mode"}},
	} {
		fields["input"] = "hello"
		var krn Kronk
		for _, stream := range []bool{false, true} {
			fields["stream"] = stream
			w := httptest.NewRecorder()
			_, err := krn.ResponseStreamingHTTP(t.Context(), w, fields)
			if !errors.Is(err, model.ErrInvalidRequest) {
				t.Errorf("fields %v stream=%v: got %v, want ErrInvalidRequest", fields, stream, err)
			}
			if w.Body.Len() != 0 || w.Header().Get("Content-Type") != "" {
				t.Errorf("invalid request committed a response: %s", w.Body.String())
			}
		}
	}
}

func TestResponsesMetadata(t *testing.T) {
	const raw = `{"input":"hello","instructions":"Be precise","user":"test-user","metadata":{"tag":"client"},"temperature":0.25,"top_p":0.75,"max_output_tokens":31,"max_tokens":7,"reasoning":{"effort":"low","summary":"auto"},"text":{"format":{"type":"json_schema","name":"answer","description":"exact","schema":{"type":"integer","enum":[9007199254740993]},"strict":false}},"tools":[{"type":"function","name":"lookup","parameters":{"type":"object"},"strict":false}]}`
	var decoded map[string]any
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.UseNumber()
	if err := dec.Decode(&decoded); err != nil {
		t.Fatal(err)
	}
	d := model.MapToModelD(decoded)
	request := d.ShallowClone()
	if _, err := convertInputToMessages(d); err != nil {
		t.Fatal(err)
	}
	if _, exists := d["instructions"]; exists {
		t.Fatal("normalization no longer consumes instructions")
	}
	ss := streamState{d: request, params: extractInputParams(request), tools: extractTools(d)}
	responses := []ResponseResponse{
		*ss.start()[0].Response,
		toChatResponseToResponses(model.ChatResponse{}, request),
	}
	for _, resp := range responses {
		data, err := json.Marshal(resp)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		dec := json.NewDecoder(strings.NewReader(string(data)))
		dec.UseNumber()
		if err := dec.Decode(&got); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"instructions", "user", "metadata", "temperature", "top_p", "max_output_tokens", "reasoning", "text", "tools"} {
			if diff := cmp.Diff(decoded[key], got[key]); diff != "" {
				t.Errorf("%s (-want +got):\n%s", key, diff)
			}
		}
	}
}

func TestResponsesReasoningWireLifecycle(t *testing.T) {
	for _, finish := range []string{model.FinishReasonStop, model.FinishReasonLength} {
		ss := streamState{d: model.D{"reasoning": model.D{"summary": "auto"}}}
		events := ss.start()
		events = append(events, ss.handleToolCalls([]model.ResponseToolCall{{ID: "call-1", Function: model.ResponseToolCallFunction{Name: "lookup", Arguments: model.ToolCallArguments{"id": 7}}}})...)
		events = append(events, ss.handleReasoningDelta("first ")...)
		events = append(events, ss.handleReasoningDelta("second")...)
		events = append(events, ss.handleTextDelta("answer")...)
		events = append(events, ss.complete(model.ChatResponse{Choices: []model.Choice{{FinishReasonPtr: &finish}}})...)
		var reasoningID string
		var deltas string
		var done bool
		for i, event := range events {
			if event.SequenceNumber != i {
				t.Errorf("event %s: sequence got %d, want %d", event.Type, event.SequenceNumber, i)
			}
			if event.Type == "response.output_item.added" && event.Item.Type == "reasoning" {
				reasoningID = event.Item.ID
			}
			if strings.HasPrefix(event.Type, "response.reasoning_summary_") {
				data, err := json.Marshal(event)
				if err != nil {
					t.Fatal(err)
				}
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(data, &fields); err != nil {
					t.Fatal(err)
				}
				if event.ItemID != reasoningID || reasoningID == "" || string(fields["output_index"]) != "1" || string(fields["summary_index"]) != "0" {
					t.Errorf("reasoning identity/index fields missing: %s", data)
				}
				if event.Type == "response.reasoning_summary_text.delta" {
					deltas += event.Delta
				}
				if event.Type == "response.reasoning_summary_text.done" {
					done = event.Text == "first second"
				}
			}
		}
		final := events[len(events)-1].Response
		if !done || deltas != "first second" || final.Reasoning.Summary == nil || *final.Reasoning.Summary != "auto" {
			t.Fatalf("reasoning lifecycle/configuration mismatch: %+v", final)
		}
		if len(final.Output) != 3 || final.Output[0].Type != "function_call" || final.Output[1].ID != reasoningID || final.Output[2].Type != "message" {
			t.Fatalf("final output no longer follows streamed indexes: %+v", final.Output)
		}
		want := []ResponseSummaryPart{{Type: "summary_text", Text: "first second"}}
		if diff := cmp.Diff(want, final.Output[1].Summary); diff != "" {
			t.Errorf("reasoning summary (-want +got):\n%s", diff)
		}
	}
}

func TestResponsesReasoningHistory(t *testing.T) {
	// JSON-decoded history can start with a reasoning item from prior output.
	// It must not turn the remaining assistant message into user content.
	var d model.D
	if err := json.Unmarshal([]byte(`{"input":[{"type":"reasoning","id":"rs_old","summary":[{"type":"summary_text","text":"thought"}]},{"type":"message","role":"assistant","content":[{"type":"output_text","text":"answer"}]},{"role":"user","content":"continue"}]}`), &d); err != nil {
		t.Fatal(err)
	}
	got, err := convertInputToMessages(d)
	if err != nil {
		t.Fatal(err)
	}
	want := []model.D{
		{"type": "message", "role": "assistant", "content": []model.D{{"type": "text", "text": "answer"}}},
		{"role": "user", "content": "continue"},
	}
	if diff := cmp.Diff(any(want), got["messages"]); diff != "" {
		t.Errorf("history (-want +got):\n%s", diff)
	}
}
