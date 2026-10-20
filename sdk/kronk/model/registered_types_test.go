package model

import (
	"encoding"
	"encoding/json"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v2"
)

func TestNumericSetSerialization(t *testing.T) {
	tests := []struct {
		name  string
		value any
		wire  string
	}{
		{"dense", ModelTypeDense, "0"},
		{"moe", ModelTypeMoE, "1"},
		{"hybrid", ModelTypeHybrid, "2"},
		{"no media", MediaTypeNone, "0"},
		{"vision", MediaTypeVision, "1"},
		{"audio", MediaTypeAudio, "2"},
		{"no channel", ChannelNone, "0"},
		{"reasoning", ChannelReasoning, "1"},
		{"answer", ChannelAnswer, "2"},
		{"tool", ChannelTool, "3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, codec := range []struct {
				name      string
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
				ending    string
			}{
				{"json", json.Marshal, json.Unmarshal, ""},
				{"yaml", yaml.Marshal, yaml.Unmarshal, "\n"},
			} {
				data, err := codec.marshal(tt.value)
				if err != nil {
					t.Fatal(err)
				}
				if string(data) != tt.wire+codec.ending {
					t.Errorf("%s: got %q, want %q", codec.name, data, tt.wire+codec.ending)
				}
				decoded := reflect.New(reflect.TypeOf(tt.value))
				if err := codec.unmarshal(data, decoded.Interface()); err != nil {
					t.Fatal(err)
				}
				if got := decoded.Elem().Interface(); got != tt.value {
					t.Errorf("%s round trip: got %v, want %v", codec.name, got, tt.value)
				}
				if err := codec.unmarshal([]byte("99"), decoded.Interface()); err == nil {
					t.Errorf("%s accepted unknown numeric value", codec.name)
				}
				if got := decoded.Elem().Interface(); got != tt.value {
					t.Errorf("%s invalid input changed value: got %v, want %v", codec.name, got, tt.value)
				}
			}
		})
	}
}

func TestRegisteredSetText(t *testing.T) {
	for _, value := range []any{
		ModelTypeDense, ModelTypeMoE, ModelTypeHybrid,
		MediaTypeNone, MediaTypeVision, MediaTypeAudio,
		ChannelNone, ChannelReasoning, ChannelAnswer, ChannelTool,
		DecisionQuestionType{}, DecisionQuestionTypeChoice, DecisionQuestionTypeScore, DecisionQuestionTypeNoul,
		IMCSessionState{}, IMCSessionStateActive, IMCSessionStateIdle, IMCSessionStateEmpty,
		NUMADisabled, NUMADistribute, NUMAIsolate, NUMANumactl, NUMAMirror,
	} {
		data, err := value.(encoding.TextMarshaler).MarshalText()
		if err != nil {
			t.Fatal(err)
		}
		decoded := reflect.New(reflect.TypeOf(value))
		parser := decoded.Interface().(encoding.TextUnmarshaler)
		if err := parser.UnmarshalText(data); err != nil {
			t.Fatal(err)
		}
		if got := decoded.Elem().Interface(); got != value {
			t.Errorf("text round trip: got %v, want %v", got, value)
		}
		if err := parser.UnmarshalText([]byte("future")); err == nil {
			t.Errorf("%T accepted unknown value", value)
		}
		if got := decoded.Elem().Interface(); got != value {
			t.Errorf("invalid input changed value: got %v, want %v", got, value)
		}
	}
}

func TestRegisteredSetZeros(t *testing.T) {
	if !(ModelType{}).Equal(ModelTypeDense) || !(ModelType{}).IsZero() {
		t.Error("model type zero must be dense")
	}
	if !(MediaType{}).Equal(MediaTypeNone) || !(MediaType{}).IsZero() {
		t.Error("media type zero must be none")
	}
	if !(Channel{}).Equal(ChannelNone) || !(Channel{}).IsZero() {
		t.Error("channel zero must be none")
	}
	if !(NUMAStrategy{}).Equal(NUMADisabled) || !(NUMAStrategy{}).IsZero() || NUMADisabled.String() != "" {
		t.Error("NUMA zero must be disabled empty")
	}
	if !(DecisionQuestionType{}).IsZero() || !(IMCSessionState{}).IsZero() {
		t.Error("question and session zeros must be unset")
	}
}

func TestStringSetSerialization(t *testing.T) {
	for _, value := range []any{
		DecisionQuestionType{}, DecisionQuestionTypeChoice, DecisionQuestionTypeScore, DecisionQuestionTypeNoul,
		IMCSessionState{}, IMCSessionStateActive, IMCSessionStateIdle, IMCSessionStateEmpty,
		NUMADisabled, NUMADistribute, NUMAIsolate, NUMANumactl, NUMAMirror,
	} {
		name := value.(interface{ String() string }).String()
		for _, codec := range []struct {
			name      string
			marshal   func(any) ([]byte, error)
			unmarshal func([]byte, any) error
		}{
			{"json", json.Marshal, json.Unmarshal},
			{"yaml", yaml.Marshal, yaml.Unmarshal},
		} {
			data, err := codec.marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var wire string
			if err := codec.unmarshal(data, &wire); err != nil {
				t.Fatal(err)
			}
			if wire != name {
				t.Errorf("%s wire: got %q, want %q", codec.name, wire, name)
			}
			decoded := reflect.New(reflect.TypeOf(value))
			if err := codec.unmarshal(data, decoded.Interface()); err != nil {
				t.Fatal(err)
			}
			if got := decoded.Elem().Interface(); got != value {
				t.Errorf("%s round trip: got %v, want %v", codec.name, got, value)
			}
		}
	}
}
