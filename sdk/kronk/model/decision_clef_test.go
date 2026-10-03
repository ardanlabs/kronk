package model

import (
	"reflect"
	"testing"

	"github.com/ardanlabs/jinja"
)

func TestClefTemplateUsesCompactJSON(t *testing.T) {
	tmpl, err := jinja.Compile(`{{ value if value is string else value | tojson(separators=[',', ':']) }}`)
	if err != nil {
		t.Fatalf("compile Clef value template: %v", err)
	}

	tests := []struct {
		name  string
		value any
		want  string
	}{
		{
			"structured value",
			map[string]any{"z": "comma, space and colon: space", "a": []any{1, 2}},
			`{"a":[1,2],"z":"comma, space and colon: space"}`,
		},
		{"string value", `{"a": 1}`, `{"a": 1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tmpl.Render(D{"value": tt.value})
			if err != nil {
				t.Fatalf("render Clef value: %v", err)
			}
			if got != tt.want {
				t.Fatalf("rendered value: got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestReplaceDecisionTemplateText(t *testing.T) {
	value := map[string]any{
		"text": "before <<clef: after",
		"nested": []any{
			map[string]any{"text": "<<clef:option>>"},
		},
	}
	want := map[string]any{
		"text": "before <<clef  after",
		"nested": []any{
			map[string]any{"text": "<<clef option>>"},
		},
	}
	if got := replaceDecisionTemplateText(value, clefMarker, "<<clef "); !reflect.DeepEqual(got, want) {
		t.Fatalf("replace marker: got %#v, want %#v", got, want)
	}
}
