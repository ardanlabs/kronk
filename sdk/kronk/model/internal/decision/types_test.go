package decision

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateRequest(t *testing.T) {
	for _, tt := range []struct {
		name     string
		protocol string
		req      DecisionRequest
		wantErr  bool
	}{
		{name: "no questions", wantErr: true},
		{name: "mixed questions", req: DecisionRequest{Questions: []DecisionQuestion{
			DecisionQuestionChoice("route", "Where?", DecisionQuestionOpt("billing", nil), DecisionQuestionOpt("support", "Technical help")),
			DecisionQuestionScore("urgency", "How urgent?", "low", "high"),
			DecisionQuestionNoul("escalate", "Escalate?", nil),
		}}},
		{name: "blank id", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionNoul(" \t", "Is it?", nil)}}, wantErr: true},
		{name: "duplicate ids", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionNoul("q", "First?", nil), DecisionQuestionNoul("q", "Second?", nil)}}, wantErr: true},
		{name: "missing instructions", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionNoul("q", nil, nil)}}, wantErr: true},
		{name: "blank instructions", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionNoul("q", " \t", nil)}}, wantErr: true},
		{name: "optional instructions", protocol: "vllm-d3", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionNoul("q", nil, nil)}}},
		{name: "no options", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionChoice("q", "Pick")}}, wantErr: true},
		{name: "unnamed option", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionChoice("q", "Pick", DecisionQuestionOpt("", nil))}}, wantErr: true},
		{name: "duplicate options", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionChoice("q", "Pick", DecisionQuestionOpt("a", nil), DecisionQuestionOpt("a", nil))}}, wantErr: true},
		{name: "one level", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionScore("q", "Rate", 0)}}, wantErr: true},
		{name: "eleven levels", req: DecisionRequest{Questions: []DecisionQuestion{DecisionQuestionScore("q", "Rate", 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)}}, wantErr: true},
		{name: "unset type", req: DecisionRequest{Questions: []DecisionQuestion{{ID: "q", Instructions: "Answer"}}}, wantErr: true},
		{name: "images belong to model validation", req: DecisionRequest{Images: [][]byte{nil}, Questions: []DecisionQuestion{DecisionQuestionNoul("q", "Is it?", nil)}}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRequest(tt.req, tt.protocol)
			if tt.wantErr {
				if !errors.Is(err, ErrDecisionRequest) {
					t.Fatalf("error: got %v, want ErrDecisionRequest", err)
				}
			} else if err != nil {
				t.Fatalf("error: got %v, want nil", err)
			}
		})
	}
}

func TestDecisionQuestionTypeRoundTrip(t *testing.T) {
	for _, name := range []string{"", "choice", "score", "noul"} {
		t.Run(name, func(t *testing.T) {
			value, err := ParseDecisionQuestionType(name)
			if err != nil {
				t.Fatalf("ParseDecisionQuestionType: %v", err)
			}
			if value.String() != name || value.IsZero() != (name == "") {
				t.Fatalf("type: got %q (zero %v), want %q", value, value.IsZero(), name)
			}
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			var decoded DecisionQuestionType
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if !decoded.Equal(value) || !MustParseDecisionQuestionType(name).Equal(value) {
				t.Errorf("type: got %q, want %q", decoded, value)
			}
		})
	}
	if _, err := ParseDecisionQuestionType("unknown"); err == nil {
		t.Error("ParseDecisionQuestionType: got nil error, want invalid type")
	}
	value := DecisionQuestionTypeChoice
	if err := value.UnmarshalText([]byte("unknown")); err == nil || value != DecisionQuestionTypeChoice {
		t.Errorf("UnmarshalText: got %q, %v, want unchanged choice and error", value, err)
	}
}

func TestDecisionValueSerialization(t *testing.T) {
	state := DecisionState(DecisionStateData("z", "<a>\u2028"), DecisionStateData("a", []any{true, nil, 1.23456789}))
	for _, tt := range []struct {
		name string
		call func() (string, error)
		want string
	}{
		{"json", func() (string, error) { return decisionJSON(state) }, "{\"z\": \"<a>\u2028\", \"a\": [true, null, 1.23456789]}"},
		{"python", func() (string, error) { return decisionPythonRepr(state) }, "{'z': '<a>\u2028', 'a': [True, None, 1.23456789]}"},
		{"decider ordered", func() (string, error) { return decisionDeciderText(state, false, false) }, "{\"z\": \"<a>\u2028\", \"a\": [true, null, 1.23457]}"},
		{"decider sorted", func() (string, error) { return decisionDeciderText(state, false, true) }, "{\"a\":[true,null,1.23457],\"z\":\"<a>\u2028\"}"},
		{"plain text", func() (string, error) { return decisionDeciderText("plain", true, true) }, "plain"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.call()
			if err != nil || got != tt.want {
				t.Errorf("serialization: got %q, %v, want %q, nil", got, err, tt.want)
			}
		})
	}
}
