package msgsapp

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/parsers/qwen"
)

func TestContentRoundTrip(t *testing.T) {
	for _, input := range []string{`""`, `"instructions"`, `[]`, `[{"type":"text","text":""}]`, `[{"type":"text","text":"first"},{"type":"text","text":"second"}]`} {
		t.Run(input, func(t *testing.T) {
			for _, value := range []any{&Content{}, &SystemContent{}} {
				if err := json.Unmarshal([]byte(input), value); err != nil {
					t.Fatal(err)
				}
				got, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				assertJSONEqual(t, string(got), input)
			}
		})
	}
	var content Content
	if err := json.Unmarshal([]byte(`[{"type":"text","text":"old"}]`), &content); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`"new"`), &content); err != nil {
		t.Fatal(err)
	}
	if content.Blocks != nil || content.Text != "new" {
		t.Errorf("reused content: got %+v, want only new text", content)
	}
}

func TestContentRejectsInvalidShapes(t *testing.T) {
	for _, input := range []string{`null`, `{}`, `12`, `true`, `[1]`, `["text"]`} {
		t.Run(input, func(t *testing.T) {
			for _, value := range []any{&Content{}, &SystemContent{}} {
				if err := json.Unmarshal([]byte(input), value); err == nil {
					t.Errorf("Unmarshal(%s, %T): got nil error, want invalid shape", input, value)
				}
			}
		})
	}
}

func TestMessagesRejectsInvalidShapes(t *testing.T) {
	for _, input := range []string{
		`{"messages":[{"role":"user"}]}`,
		`{"messages":[{"role":"user","content":null}]}`,
		`{"messages":[{"role":"user","content":{}}]}`,
		`{"messages":[{"role":"user","content":[{"type":"text"}]}]}`,
		`{"system":[{"type":"text","text":null}]}`,
		`{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":{}}]}]}`,
		`{"tools":[{"name":"f","input_schema":[]}]}`,
		`{"tool_choice":"auto"}`,
	} {
		t.Run(input, func(t *testing.T) {
			var req MessagesRequest
			if err := json.Unmarshal([]byte(input), &req); err == nil {
				t.Errorf("Unmarshal: got nil error for %s, want invalid shape", input)
			}
		})
	}
}

func TestContentDecodingPreservesNumbers(t *testing.T) {
	input := `[{"type":"tool_use","id":"call","name":"lookup","input":{"n":9007199254740993,"nested":[{"n":9223372036854775807}]}}]`
	for _, value := range []any{&Content{}, &SystemContent{}} {
		if err := json.Unmarshal([]byte(input), value); err != nil {
			t.Fatal(err)
		}
		var blocks []ContentBlock
		switch v := value.(type) {
		case *Content:
			blocks = v.Blocks
		case *SystemContent:
			blocks = v.Blocks
		}
		if got := blocks[0].Input.(map[string]any)["n"]; got != json.Number("9007199254740993") {
			t.Errorf("number: got %v (%T), want exact json.Number", got, got)
		}
		got, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, string(got), input)
	}
}

func TestMessagesSchemaAndHistory(t *testing.T) {
	req := decodeMessagesRequest(t, `{
		"model":"test", "max_tokens":32,
		"system":[{"type":"text","text":"first"},{"type":"text","text":"second"}],
		"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"call","name":"lookup","input":{"id":9007199254740993}}]}],
		"tools":[{"name":"lookup","input_schema":{
			"type":"object", "properties":{"id":{"type":"integer","minimum":9007199254740993},"enabled":{"type":"boolean"}},
			"required":["id"],"additionalProperties":false,"$defs":{"choice":{"enum":[9007199254740993]}},
			"allOf":[{"properties":{"id":{"maximum":9223372036854775807}}}]
		}}]
	}`)
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	d := toOpenAI(req)
	messages := d["messages"].([]model.D)
	if got := messages[0]["content"]; got != "firstsecond" {
		t.Errorf("system: got %v, want firstsecond", got)
	}
	call := messages[1]["tool_calls"].([]model.D)[0]["function"].(model.D)
	if got := call["arguments"]; got != `{"id":9007199254740993}` {
		t.Errorf("arguments: got %v, want exact large integer", got)
	}
	tools := d["tools"].([]model.D)
	parameters := tools[0]["function"].(model.D)["parameters"].(model.D)
	properties := parameters["properties"].(model.D)
	if got := properties["id"].(model.D)["minimum"]; got != json.Number("9007199254740993") {
		t.Errorf("minimum: got %v (%T), want exact json.Number", got, got)
	}
	if got := parameters["allOf"].([]any)[0].(model.D)["properties"].(model.D)["id"].(model.D)["maximum"]; got != json.Number("9223372036854775807") {
		t.Errorf("maximum: got %v, want exact large integer", got)
	}
	got, err := json.Marshal(parameters)
	if err != nil {
		t.Fatal(err)
	}
	want, err := json.Marshal(req.Tools[0].InputSchema)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, string(got), string(want))

	// Exercise Qwen's real schema consumer, not just its expected Go shape.
	calls := (qwen.Parser{}).ToolCallWithSchema(t.Context(), nil,
		"<function=lookup>\n<parameter=id>\n9007199254740993\n</parameter>\n<parameter=enabled>\ntrue\n</parameter>\n</function>", tools)
	if len(calls) != 1 {
		t.Fatalf("calls: got %d, want 1", len(calls))
	}
	args := calls[0].Function.Arguments
	if args["id"] != json.Number("9007199254740993") || args["enabled"] != true {
		t.Errorf("Qwen arguments: got %#v, want exact integer and boolean", args)
	}
}

func TestToolResultTranslation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		isError bool
		want    any
	}{
		{name: "text", content: `"ok"`, want: "ok"},
		{name: "error text", content: `"denied"`, isError: true, want: "Tool execution failed: denied"},
		{name: "blocks", content: `[{"type":"text","text":"ok"},{"type":"image","source":{"type":"url","url":"https://example.com/image.png"}}]`, want: []model.D{{"type": "text", "text": "ok"}, {"type": "image_url", "image_url": model.D{"url": "https://example.com/image.png"}}}},
		{name: "error blocks", content: `[{"type":"text","text":"denied"}]`, isError: true, want: []model.D{{"type": "text", "text": "Tool execution failed: "}, {"type": "text", "text": "denied"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			errorJSON := "false"
			if tc.isError {
				errorJSON = "true"
			}
			req := decodeMessagesRequest(t, `{"model":"test","max_tokens":10,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"call","content":`+tc.content+`,"is_error":`+errorJSON+`},{"type":"text","text":"continue"}]}]}`)
			req.Messages = append([]Message{{Role: "assistant", Content: Content{Blocks: []ContentBlock{{Type: "tool_use", ID: "call", Name: "lookup", Input: model.D{}}}}}}, req.Messages...)
			if err := req.Validate(); err != nil {
				t.Fatal(err)
			}
			wire, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			roundTrip := decodeMessagesRequest(t, string(wire))
			if got := roundTrip.Messages[1].Content.Blocks[0].IsError; got != tc.isError {
				t.Errorf("is_error: got %v, want %v", got, tc.isError)
			}
			messages := toOpenAI(roundTrip)["messages"].([]model.D)[1:]
			if len(messages) != 2 || messages[0]["role"] != "tool" || messages[0]["tool_call_id"] != "call" || messages[1]["role"] != "user" || messages[1]["content"] != "continue" {
				t.Fatalf("messages: got %#v, want tool result followed by user content", messages)
			}
			if !reflect.DeepEqual(messages[0]["content"], tc.want) {
				t.Errorf("tool content: got %#v, want %#v", messages[0]["content"], tc.want)
			}
		})
	}
}

func TestMultipleToolResultsRetainOrder(t *testing.T) {
	req := decodeMessagesRequest(t, `{"model":"test","max_tokens":1,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"first"},{"type":"tool_result","tool_use_id":"b","content":"second"},{"type":"text","text":"continue"}]}]}`)
	req.Messages = append([]Message{{Role: "assistant", Content: Content{Blocks: []ContentBlock{
		{Type: "tool_use", ID: "a", Name: "lookup", Input: model.D{}},
		{Type: "tool_use", ID: "b", Name: "lookup", Input: model.D{}},
	}}}}, req.Messages...)
	if err := req.Validate(); err != nil {
		t.Fatal(err)
	}
	want := []model.D{{"role": "tool", "tool_call_id": "a", "content": "first"}, {"role": "tool", "tool_call_id": "b", "content": "second"}, {"role": "user", "content": "continue"}}
	if got := toOpenAI(req)["messages"].([]model.D)[1:]; !reflect.DeepEqual(got, want) {
		t.Errorf("messages: got %#v, want %#v", got, want)
	}
}

func TestMessagesValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "basic", input: `{"messages":[{"role":"user","content":"hello"}]}`},
		{name: "sampling lower bounds", input: `{"temperature":0,"top_p":0,"top_k":0,"messages":[{"role":"user","content":"hello"}]}`},
		{name: "sampling upper bounds", input: `{"temperature":1,"top_p":1,"messages":[{"role":"user","content":"hello"}]}`},
		{name: "negative temperature", input: `{"temperature":-0.1,"messages":[{"role":"user","content":"hello"}]}`, wantErr: true},
		{name: "high temperature", input: `{"temperature":1.1,"messages":[{"role":"user","content":"hello"}]}`, wantErr: true},
		{name: "negative top p", input: `{"top_p":-0.1,"messages":[{"role":"user","content":"hello"}]}`, wantErr: true},
		{name: "high top p", input: `{"top_p":1.1,"messages":[{"role":"user","content":"hello"}]}`, wantErr: true},
		{name: "negative top k", input: `{"top_k":-1,"messages":[{"role":"user","content":"hello"}]}`, wantErr: true},
		{name: "base64", input: `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGk="}}]}]}`},
		{name: "negative tokens", input: `{"max_tokens":-1,"messages":[{"role":"user","content":"hi"}]}`, wantErr: true},
		{name: "role", input: `{"messages":[{"role":"system","content":"hi"}]}`, wantErr: true},
		{name: "system block", input: `{"system":[{"type":"image"}],"messages":[{"role":"user","content":"hi"}]}`, wantErr: true},
		{name: "unknown block", input: `{"messages":[{"role":"user","content":[{"type":"document"}]}]}`, wantErr: true},
		{name: "missing image source", input: `{"messages":[{"role":"user","content":[{"type":"image"}]}]}`, wantErr: true},
		{name: "unknown source", input: `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"file"}}]}]}`, wantErr: true},
		{name: "bad media", input: `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"text/plain","data":"aGk="}}]}]}`, wantErr: true},
		{name: "bad data", input: `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"!"}}]}]}`, wantErr: true},
		{name: "bad url", input: `{"messages":[{"role":"user","content":[{"type":"image","source":{"type":"url","url":"file:///tmp/a"}}]}]}`, wantErr: true},
		{name: "assistant image", input: `{"messages":[{"role":"assistant","content":[{"type":"image","source":{"type":"url","url":"https://example.com/a"}}]}]}`, wantErr: true},
		{name: "tool identity", input: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","name":"f","input":{}}]}]}`, wantErr: true},
		{name: "tool name", input: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c","input":{}}]}]}`, wantErr: true},
		{name: "tool input missing", input: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"f"}]}]}`, wantErr: true},
		{name: "tool input array", input: `{"messages":[{"role":"assistant","content":[{"type":"tool_use","id":"c","name":"f","input":[]}]}]}`, wantErr: true},
		{name: "user tool use", input: `{"messages":[{"role":"user","content":[{"type":"tool_use","id":"c","name":"f","input":{}}]}]}`, wantErr: true},
		{name: "result identity", input: `{"messages":[{"role":"user","content":[{"type":"tool_result","content":"ok"}]}]}`, wantErr: true},
		{name: "assistant result", input: `{"messages":[{"role":"assistant","content":[{"type":"tool_result","tool_use_id":"c","content":"ok"}]}]}`, wantErr: true},
		{name: "result nested block", input: `{"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"c","content":[{"type":"tool_result","tool_use_id":"d"}]}]}]}`, wantErr: true},
		{name: "reverse order", input: `{"messages":[{"role":"user","content":[{"type":"text","text":"hi"},{"type":"tool_result","tool_use_id":"c","content":"ok"}]}]}`, wantErr: true},
		{name: "schema missing", input: `{"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"f"}]}`, wantErr: true},
		{name: "schema type", input: `{"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"f","input_schema":{"type":"array"}}]}`, wantErr: true},
		{name: "tool definition name", input: `{"messages":[{"role":"user","content":"hi"}],"tools":[{"input_schema":{"type":"object"}}]}`, wantErr: true},
		{name: "hosted tool", input: `{"messages":[{"role":"user","content":"hi"}],"tools":[{"type":"web_search_20250305","name":"web_search","input_schema":{"type":"object"}}]}`, wantErr: true},
		{name: "thinking", input: `{"messages":[{"role":"user","content":"hi"}],"thinking":{"type":"enabled","budget_tokens":1024}}`, wantErr: true},
		{name: "output config", input: `{"messages":[{"role":"user","content":"hi"}],"output_config":{"effort":"high"}}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.input), &fields); err != nil {
				t.Fatal(err)
			}
			fields["model"] = json.RawMessage(`"test"`)
			if _, exists := fields["max_tokens"]; !exists {
				fields["max_tokens"] = json.RawMessage(`10`)
			}
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			req := decodeMessagesRequest(t, string(data))
			if err := req.Validate(); (err != nil) != tc.wantErr {
				t.Errorf("Validate: got %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
	req := MessagesRequest{Model: "test", MaxTokens: 1}
	if err := req.Validate(); !errors.Is(err, model.ErrMessagesMissing) {
		t.Errorf("missing messages: got %v, want ErrMessagesMissing", err)
	}
	req.StopSequences = []string{"stop"}
	if err := req.Validate(); err == nil || err.Error() != "stop_sequences is not supported" {
		t.Errorf("stop_sequences: got %v, want existing unsupported error", err)
	}
}

func TestToolChoiceMapping(t *testing.T) {
	for _, tc := range []struct {
		input   string
		want    any
		wantErr bool
	}{
		{input: `{"type":"auto"}`, want: "auto"},
		{input: `{"type":"none"}`, want: "none"},
		{input: `{"type":"any"}`, want: "required"},
		{input: `{"type":"tool","name":"f"}`, want: model.D{"type": "function", "function": model.D{"name": "f"}}},
		{input: `{"type":"auto","disable_parallel_tool_use":false}`, want: "auto"},
		{input: `{"type":"auto","disable_parallel_tool_use":true}`, wantErr: true},
		{input: `{"type":"tool","name":"absent"}`, wantErr: true},
		{input: `{"type":"tool"}`, wantErr: true},
		{input: `{"type":"auto","name":"f"}`, wantErr: true},
		{input: `{"type":"required"}`, wantErr: true},
	} {
		t.Run(tc.input, func(t *testing.T) {
			req := decodeMessagesRequest(t, `{"model":"test","max_tokens":1,"messages":[{"role":"user","content":"hi"}],"tools":[{"name":"f","input_schema":{"type":"object"}}],"tool_choice":`+tc.input+`}`)
			if err := req.Validate(); (err != nil) != tc.wantErr {
				t.Fatalf("Validate: got %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr {
				return
			}
			if got := toOpenAI(req)["tool_choice"]; !reflect.DeepEqual(got, tc.want) {
				t.Errorf("tool_choice: got %#v, want %#v", got, tc.want)
			}
		})
	}
	var req MessagesRequest
	if err := json.Unmarshal([]byte(`{"tool_choice":{"type":"auto","unknown_control":true}}`), &req); err == nil {
		t.Error("unknown tool choice control: got nil error, want rejection")
	}
	for _, choice := range []string{"any", "tool"} {
		req = MessagesRequest{Model: "test", MaxTokens: 1, Messages: []Message{{Role: "user", Content: Content{Text: "hi"}}}, ToolChoice: &ToolChoice{Type: choice}}
		if choice == "tool" {
			req.ToolChoice.Name = "f"
		}
		if err := req.Validate(); err == nil {
			t.Errorf("tool choice %s without tools: got nil error, want rejection", choice)
		}
	}
}

func TestContentBlockMetadataJSON(t *testing.T) {
	for _, tc := range []struct {
		input ContentBlockMetadata
		want  string
	}{
		{input: ContentBlockMetadata{Type: "text"}, want: `{"type":"text","text":""}`},
		{input: ContentBlockMetadata{Type: "tool_use", Text: "ignored", ID: "c", Name: "f", Input: model.D{}}, want: `{"type":"tool_use","id":"c","name":"f","input":{}}`},
	} {
		data, err := json.Marshal(tc.input)
		if err != nil {
			t.Fatal(err)
		}
		assertJSONEqual(t, string(data), tc.want)
	}
}

func TestMessagesEmptyToolInputIsObject(t *testing.T) {
	resp := toMessagesResponse(model.ChatResponse{Choices: []model.Choice{{Message: &model.ResponseMessage{
		ToolCalls: []model.ResponseToolCall{{ID: "empty", Function: model.ResponseToolCallFunction{Name: "ping"}}},
	}}}})
	data, err := json.Marshal(resp.Content)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, string(data), `[{"type":"tool_use","id":"empty","name":"ping","input":{}}]`)
}

func TestMessagesToolResultReferences(t *testing.T) {
	assistant := `{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"lookup","input":{}},{"type":"tool_use","id":"b","name":"lookup","input":{}}]}`
	for _, tt := range []struct {
		name     string
		messages string
		wantErr  bool
	}{
		{"valid reordered results", assistant + `,{"role":"user","content":[{"type":"tool_result","tool_use_id":"b","content":"B"},{"type":"tool_result","tool_use_id":"a","content":"A"}]}`, false},
		{"dangling result", `{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"A"}]}`, true},
		{"duplicate result", assistant + `,{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"A"},{"type":"tool_result","tool_use_id":"a","content":"A"}]}`, true},
		{"missing result", assistant + `,{"role":"user","content":[{"type":"tool_result","tool_use_id":"a","content":"A"}]}`, true},
		{"intervening user text", assistant + `,{"role":"user","content":"continue"}`, true},
		{"intervening assistant", assistant + `,{"role":"assistant","content":"continue"}`, true},
		{"duplicate call", `{"role":"assistant","content":[{"type":"tool_use","id":"a","name":"lookup","input":{}},{"type":"tool_use","id":"a","name":"lookup","input":{}}]}`, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := decodeMessagesRequest(t, `{"model":"test","max_tokens":10,"messages":[`+tt.messages+`]}`)
			if err := req.Validate(); (err != nil) != tt.wantErr {
				t.Errorf("Validate: got %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func decodeMessagesRequest(t *testing.T, input string) MessagesRequest {
	t.Helper()
	var req MessagesRequest
	if err := json.Unmarshal([]byte(input), &req); err != nil {
		t.Fatal(err)
	}
	return req
}

func assertJSONEqual(t *testing.T, got, want string) {
	t.Helper()
	decode := func(input string) any {
		var value any
		dec := json.NewDecoder(strings.NewReader(input))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			t.Fatal(err)
		}
		return value
	}
	if !reflect.DeepEqual(decode(got), decode(want)) {
		t.Errorf("JSON: got %s, want %s", got, want)
	}
}
