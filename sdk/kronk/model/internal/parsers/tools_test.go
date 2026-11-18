package parsers

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestToolCallArgumentsJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		input   string
		want    ToolCallArguments
		wantErr bool
	}{
		{"object", `{"n":9007199254740993}`, ToolCallArguments{"n": json.Number("9007199254740993")}, false},
		{"string", `"{\"n\":1e100}"`, ToolCallArguments{"n": json.Number("1e100")}, false},
		{"null", `null`, nil, false},
		{"empty string", `""`, nil, false},
		{"trailing object in string", `"{} {}"`, nil, true},
		{"array", `[]`, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got ToolCallArguments
			err := json.Unmarshal([]byte(tc.input), &got)
			if (err != nil) != tc.wantErr {
				t.Fatalf("error: got %v, want error %v", err, tc.wantErr)
			}
			if !tc.wantErr && !reflect.DeepEqual(got, tc.want) {
				t.Errorf("arguments: got %#v, want %#v", got, tc.want)
			}
		})
	}
	data, err := json.Marshal(ToolCallArguments(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `"{}"` {
		t.Errorf("nil arguments: got %s, want quoted empty object", data)
	}
}

func TestMapToD(t *testing.T) {
	input := map[string]any{
		"tools": []any{map[string]any{"function": map[string]any{"name": "lookup"}}},
		"mixed": []any{map[string]any{"type": "string"}, "scalar"},
		"empty": []any{},
	}
	want := D{
		"tools": []D{{"function": D{"name": "lookup"}}},
		"mixed": []any{D{"type": "string"}, "scalar"},
		"empty": []D{},
	}
	if got := MapToD(input); !reflect.DeepEqual(got, want) {
		t.Errorf("normalized schema: got %#v, want %#v", got, want)
	}
}

func TestChannelValues(t *testing.T) {
	for i, channel := range []Channel{ChannelNone, ChannelReasoning, ChannelAnswer, ChannelTool} {
		if uint8(channel) != uint8(i) {
			t.Errorf("channel %s: got %d, want %d", channel, uint8(channel), i)
		}
		if got, want := channel.String(), []string{"none", "reasoning", "answer", "tool"}[i]; got != want {
			t.Errorf("channel name: got %q, want %q", got, want)
		}
	}
	if (Result{}) != (Result{Channel: ChannelNone}) {
		t.Error("zero Result must have ChannelNone")
	}
}
