package vram_test

import (
	"encoding/json"
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/vram"
	yaml "go.yaml.in/yaml/v2"
	yamlv3 "go.yaml.in/yaml/v3"
)

func TestRegisteredFitStatuses(t *testing.T) {
	for _, tt := range []struct {
		value vram.FitStatus
		want  string
	}{
		{vram.FitStatus{}, ""},
		{vram.FitStatusUnknown, "unknown"},
		{vram.FitStatusFits, "fits"},
		{vram.FitStatusTight, "tight"},
		{vram.FitStatusDoesNotFit, "does_not_fit"},
	} {
		t.Run(tt.want, func(t *testing.T) {
			if tt.value.String() != tt.want || tt.value.IsZero() != (tt.want == "") || !vram.MustParseFitStatus(tt.want).Equal(tt.value) {
				t.Fatalf("status: got %v, want %q", tt.value, tt.want)
			}
			text, err := tt.value.MarshalText()
			if err != nil || string(text) != tt.want {
				t.Fatalf("text: got %q, %v", text, err)
			}
			var decoded vram.FitStatus
			if err := decoded.UnmarshalText(text); err != nil || !decoded.Equal(tt.value) {
				t.Fatalf("text round trip: got %v, %v, want %v, nil", decoded, err, tt.value)
			}
			if err := decoded.UnmarshalText([]byte("invalid")); err == nil || !decoded.Equal(tt.value) {
				t.Fatal("invalid text did not fail atomically")
			}
			for _, codec := range []struct {
				name      string
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
			}{
				{"json", json.Marshal, json.Unmarshal},
				{"yaml2", yaml.Marshal, yaml.Unmarshal},
				{"yaml3", yamlv3.Marshal, yamlv3.Unmarshal},
			} {
				t.Run(codec.name, func(t *testing.T) {
					data, err := codec.marshal(tt.value)
					if err != nil {
						t.Fatal(err)
					}
					var wire string
					if err := codec.unmarshal(data, &wire); err != nil || wire != tt.want {
						t.Fatalf("wire: got %q, %v, want %q, nil", wire, err, tt.want)
					}
					var decoded vram.FitStatus
					if err := codec.unmarshal(data, &decoded); err != nil || !decoded.Equal(tt.value) {
						t.Fatalf("round trip: got %v, %v, want %v, nil", decoded, err, tt.value)
					}
					invalid, err := codec.marshal("invalid")
					if err != nil {
						t.Fatal(err)
					}
					if err := codec.unmarshal(invalid, &decoded); err == nil || !decoded.Equal(tt.value) {
						t.Fatal("invalid wire value did not fail atomically")
					}
				})
			}
		})
	}
}

func TestFitStatusOmission(t *testing.T) {
	type values struct {
		Status vram.FitStatus `json:"status,omitzero" yaml:"status,omitempty"`
	}
	for _, marshal := range []func(any) ([]byte, error){json.Marshal, yaml.Marshal, yamlv3.Marshal} {
		data, err := marshal(values{})
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]any
		if err := yaml.Unmarshal(data, &fields); err != nil || len(fields) != 0 {
			t.Fatalf("zero status: got %s, %v, want empty object", data, err)
		}
		data, err = marshal(values{Status: vram.FitStatusUnknown})
		if err != nil {
			t.Fatal(err)
		}
		if err := yaml.Unmarshal(data, &fields); err != nil || fields["status"] != "unknown" {
			t.Fatalf("unknown status: got %s, %v, want explicit unknown", data, err)
		}
	}
}

func TestMustParseFitStatusRejectsUnknownValue(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParseFitStatus did not panic")
		}
	}()
	vram.MustParseFitStatus("invalid")
}
