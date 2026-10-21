package chatapi_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk"
	"github.com/ardanlabs/kronk/sdk/kronk/model"
	"github.com/ardanlabs/kronk/sdk/kronk/tests/testlib"
)

// These synthetic checks exercise the shared SDK validators without starting
// the native model suites or the API server.
func TestToolResponseValidators(t *testing.T) {
	for _, tt := range []struct {
		name      string
		function  string
		arguments map[string]any
		count     int
		wantErr   bool
	}{
		{name: "valid", function: "get_weather", arguments: map[string]any{"location": "London, England"}, count: 1},
		{name: "wrong function", function: "get_weather_forecast", arguments: map[string]any{"location": "London"}, count: 1, wantErr: true},
		{name: "wrong case", function: "GET_WEATHER", arguments: map[string]any{"location": "London"}, count: 1, wantErr: true},
		{name: "missing argument", function: "get_weather", arguments: map[string]any{"city": "London"}, count: 1, wantErr: true},
		{name: "wrong type", function: "get_weather", arguments: map[string]any{"location": 42}, count: 1, wantErr: true},
		{name: "null argument", function: "get_weather", arguments: map[string]any{"location": nil}, count: 1, wantErr: true},
		{name: "wrong value", function: "get_weather", arguments: map[string]any{"location": "Paris", "note": "London"}, count: 1, wantErr: true},
		{name: "missing calls", count: 0, wantErr: true},
		{name: "extra call", function: "get_weather", arguments: map[string]any{"location": "London"}, count: 2, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var calls []model.ResponseToolCall
			var output []kronk.ResponseOutputItem
			arguments, err := json.Marshal(tt.arguments)
			if err != nil {
				t.Fatal(err)
			}
			for range tt.count {
				calls = append(calls, model.ResponseToolCall{
					Type: "function",
					Function: model.ResponseToolCallFunction{
						Name: tt.function, Arguments: model.ToolCallArguments(tt.arguments),
					},
				})
				output = append(output, kronk.ResponseOutputItem{
					Type: "function_call", Name: tt.function, Arguments: new(string(arguments)),
				})
			}
			resp := model.ChatResponse{
				ID: "completion", Created: 1, Model: "fixture", Object: model.ObjectChatTextFinal,
				Choices: []model.Choice{{
					FinishReasonPtr: new(model.FinishReasonTool),
					Message:         &model.ResponseMessage{Role: "assistant", ToolCalls: calls},
				}},
			}
			response := kronk.ResponseResponse{
				ID: "response", CreatedAt: 1, Model: "fixture", Object: "response",
				Status: "completed", Output: output,
			}
			var acc testlib.StreamAccumulator
			acc.Accumulate(resp)
			results := []struct {
				name string
				err  error
			}{
				{"chat", testlib.TestChatResponse(resp, "fixture", model.ObjectChatTextFinal, "London", "get_weather", "location", false, false).Err},
				{"stream", testlib.TestStreamingToolCall(&acc, resp, "London", "get_weather", "location").Err},
				{"responses", testlib.TestResponseResponse(response, "fixture", "London", "get_weather", "location")},
			}
			for _, result := range results {
				if (result.err != nil) != tt.wantErr {
					t.Errorf("%s: got error %v, want error %t", result.name, result.err, tt.wantErr)
				}
			}
			v := validateResponse(&resp, false).hasToolCalls("get_weather", "location", "London")
			if (len(v.errors) != 0) != tt.wantErr {
				t.Errorf("API: got errors %v, want error %t", v.errors, tt.wantErr)
			}
		})
	}
}

func TestResponseToolArgumentsJSON(t *testing.T) {
	for _, arguments := range []*string{nil, new(`{"location":"London"`), new(`"London"`), new(`null`), new(`[]`)} {
		resp := kronk.ResponseResponse{
			ID: "response", CreatedAt: 1, Model: "fixture", Object: "response", Status: "completed",
			Output: []kronk.ResponseOutputItem{{Type: "function_call", Name: "get_weather", Arguments: arguments}},
		}
		if err := testlib.TestResponseResponse(resp, "fixture", "London", "get_weather", "location"); err == nil {
			t.Errorf("got no error for arguments %v", arguments)
		}
	}
}

func TestToolFinishReasonValidators(t *testing.T) {
	for _, reason := range []string{"stop", "length", "error", "tool", ""} {
		t.Run(reason, func(t *testing.T) {
			resp := model.ChatResponse{
				ID: "completion", Created: 1, Model: "fixture", Object: model.ObjectChatTextFinal,
				Choices: []model.Choice{{
					FinishReasonPtr: new(reason),
					Message: &model.ResponseMessage{Role: "assistant", Content: "London", ToolCalls: []model.ResponseToolCall{{
						Function: model.ResponseToolCallFunction{Name: "get_weather", Arguments: model.ToolCallArguments{"location": "London"}},
					}}},
				}},
			}
			if result := testlib.TestChatResponse(resp, "fixture", model.ObjectChatTextFinal, "London", "get_weather", "location", false, false); result.Err == nil {
				t.Errorf("chat: got no error for finish reason %q", reason)
			}
			var acc testlib.StreamAccumulator
			acc.Accumulate(resp)
			if result := testlib.TestStreamingToolCall(&acc, resp, "London", "get_weather", "location"); result.Err == nil {
				t.Errorf("stream: got no error for finish reason %q", reason)
			}
		})
	}
}

func TestGrammarValidators(t *testing.T) {
	for _, tt := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "valid", content: `{"languages":[{"name":"Go","year":2009},{"name":"Python","year":1991},{"name":"Rust","year":2010}]}`},
		{name: "other valid languages", content: `{"languages":[{"year":1972,"name":"C"},{"name":"Java","year":1995},{"name":"Ruby","year":1995}]}`},
		{name: "malformed", content: `{"languages":`, wantErr: true},
		{name: "null", content: `null`, wantErr: true},
		{name: "array", content: `[]`, wantErr: true},
		{name: "missing field", content: `{"other":3}`, wantErr: true},
		{name: "empty list", content: `{"languages":[]}`, wantErr: true},
		{name: "wrong count", content: `{"languages":[{"name":"Go","year":2009}]}`, wantErr: true},
		{name: "too many languages", content: `{"languages":[{"name":"Go","year":2009},{"name":"Python","year":1991},{"name":"Rust","year":2010},{"name":"C","year":1972}]}`, wantErr: true},
		{name: "wrong element type", content: `{"languages":["Go","Python","Rust"]}`, wantErr: true},
		{name: "empty name", content: `{"languages":[{"name":" ","year":2009},{"name":"Python","year":1991},{"name":"Rust","year":2010}]}`, wantErr: true},
		{name: "zero year", content: `{"languages":[{"name":"Go","year":0},{"name":"Python","year":1991},{"name":"Rust","year":2010}]}`, wantErr: true},
		{name: "string year", content: `{"languages":[{"name":"Go","year":"2009"},{"name":"Python","year":1991},{"name":"Rust","year":2010}]}`, wantErr: true},
		{name: "missing year", content: `{"languages":[{"name":"Go"},{"name":"Python","year":1991},{"name":"Rust","year":2010}]}`, wantErr: true},
		{name: "fractional year", content: `{"languages":[{"name":"Go","year":2009.5},{"name":"Python","year":1991},{"name":"Rust","year":2010}]}`, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			resp := model.ChatResponse{
				ID: "completion", Model: "fixture", Object: model.ObjectChatTextFinal,
				Choices: []model.Choice{{Message: &model.ResponseMessage{Content: tt.content}}},
			}
			for _, result := range []struct {
				name string
				err  error
			}{
				{"chat", testlib.TestGrammarJSONResponse(resp, "fixture").Err},
				{"stream", testlib.TestGrammarStreamingContent(tt.content, "fixture").Err},
			} {
				if (result.err != nil) != tt.wantErr {
					t.Errorf("%s: got error %v, want error %t", result.name, result.err, tt.wantErr)
				}
			}
			v := validateResponse(&resp, false).hasGrammarJSON()
			if (len(v.errors) != 0) != tt.wantErr {
				t.Errorf("API: got errors %v, want error %t", v.errors, tt.wantErr)
			}

			// Exercise the real API table's full-stream callback, with JSON split
			// inside tokens rather than only at object boundaries.
			var events []json.RawMessage
			for _, fragment := range []string{tt.content[:len(tt.content)/2], tt.content[len(tt.content)/2:]} {
				data, err := json.Marshal(fragment)
				if err != nil {
					t.Fatal(err)
				}
				events = append(events, json.RawMessage(fmt.Sprintf(`{"id":"completion","choices":[{"delta":{"content":%s},"finish_reason":null}],"usage":null}`, data)))
			}
			events = append(events,
				json.RawMessage(`{"id":"completion","choices":[{"delta":{},"finish_reason":"stop"}],"usage":null}`),
				json.RawMessage(`{"id":"completion","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30,"tokens_per_second":1}}`),
			)
			result := chatGrammarStreamQwen3(t, nil)[0].StreamCmpFunc(events)
			if (result != "") != tt.wantErr {
				t.Errorf("API stream: got error %q, want error %t", result, tt.wantErr)
			}
		})
	}
}

func TestToolCallStreamValidator(t *testing.T) {
	for _, tt := range []struct {
		name      string
		function  string
		fragments []string
		wantErr   bool
	}{
		{name: "fragmented arguments", function: "get_weather", fragments: []string{`{"loc`, `ation":"Lon`, `don, England"}`}},
		{name: "wrong function", function: "get_weather_forecast", fragments: []string{`{"location":"London"}`}, wantErr: true},
		{name: "wrong case", function: "GET_WEATHER", fragments: []string{`{"location":"London"}`}, wantErr: true},
		{name: "malformed arguments", function: "get_weather", fragments: []string{`{"location":"London"`}, wantErr: true},
		{name: "missing argument", function: "get_weather", fragments: []string{`{"city":"London"}`}, wantErr: true},
		{name: "wrong type", function: "get_weather", fragments: []string{`{"location":42}`}, wantErr: true},
		{name: "wrong value", function: "get_weather", fragments: []string{`{"location":"Paris","note":"London"}`}, wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var events []json.RawMessage
			for i, fragment := range tt.fragments {
				function := ""
				id := ""
				if i == 0 {
					function = tt.function
					id = "call-1"
				}
				data, err := json.Marshal(model.D{
					"id": "completion",
					"choices": []model.D{{"delta": model.D{"tool_calls": []model.D{{
						"index": 0, "id": id, "type": "function",
						"function": model.D{"name": function, "arguments": fragment},
					}}}}},
				})
				if err != nil {
					t.Fatal(err)
				}
				events = append(events, data)
			}
			events = append(events, json.RawMessage(`{"id":"completion","choices":[{"delta":{},"finish_reason":"tool_calls"}]}`))
			if result := validateToolCallStream(events, "get_weather", "location", "London"); (result != "") != tt.wantErr {
				t.Errorf("got error %q, want error %t", result, tt.wantErr)
			}
		})
	}
}
