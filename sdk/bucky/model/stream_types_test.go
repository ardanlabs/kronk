package model_test

import (
	"encoding"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/ardanlabs/kronk/sdk/bucky/model"
	"go.yaml.in/yaml/v2"
)

func TestEventKind(t *testing.T) {
	testStreamType(t, []model.EventKind{model.EventPartial, model.EventFinal, model.EventReset, model.EventError},
		[]string{"partial", "final", "reset", "error"}, model.ParseEventKind, model.MustParseEventKind)
}

func TestSampleEncoding(t *testing.T) {
	testStreamType(t, []model.SampleEncoding{model.Int16LE, model.Float32LE},
		[]string{"int16le", "float32le"}, model.ParseSampleEncoding, model.MustParseSampleEncoding)
}

func testStreamType[T interface {
	Int() int
	String() string
	IsZero() bool
	Equal(T) bool
	MarshalText() ([]byte, error)
}](t *testing.T, values []T, names []string, parse func(string) (T, error), mustParse func(string) T) {
	t.Helper()
	var zero T
	if !zero.Equal(values[0]) || !zero.IsZero() {
		t.Fatalf("zero: got %v, want %v", zero, values[0])
	}
	if got, err := parse(""); err != nil || !got.Equal(zero) {
		t.Fatalf("empty: got %v/%v, want %v", got, err, zero)
	}
	for i, value := range values {
		t.Run(names[i], func(t *testing.T) {
			if value.Int() != i || value.String() != names[i] || value.IsZero() != (i == 0) {
				t.Fatalf("got %q/%d, want %q/%d", value.String(), value.Int(), names[i], i)
			}
			for _, input := range []string{names[i], strconv.Itoa(i)} {
				got, err := parse(input)
				if err != nil || !got.Equal(value) || !mustParse(input).Equal(value) {
					t.Fatalf("parse %q: got %v/%v, want %v", input, got, err, value)
				}
			}
			text, err := value.MarshalText()
			if err != nil || string(text) != names[i] {
				t.Fatalf("text: got %q/%v, want %q", text, err, names[i])
			}
			var fromText T
			if err := any(&fromText).(encoding.TextUnmarshaler).UnmarshalText(text); err != nil || !fromText.Equal(value) {
				t.Fatalf("text round trip: got %v/%v, want %v", fromText, err, value)
			}
			for _, codec := range []struct {
				name      string
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
				want      string
			}{
				{"json", json.Marshal, json.Unmarshal, strconv.Itoa(i)},
				{"yaml", yaml.Marshal, yaml.Unmarshal, strconv.Itoa(i) + "\n"},
			} {
				data, err := codec.marshal(value)
				if err != nil || string(data) != codec.want {
					t.Fatalf("%s: got %q/%v, want %q", codec.name, data, err, codec.want)
				}
				var got T
				if err := codec.unmarshal(data, &got); err != nil || !got.Equal(value) {
					t.Fatalf("%s round trip: got %v/%v, want %v", codec.name, got, err, value)
				}
				got = value
				if err := codec.unmarshal([]byte("99"), &got); err == nil || !got.Equal(value) {
					t.Fatalf("%s invalid value: got %v/%v, want unchanged/error", codec.name, got, err)
				}
			}
		})
	}
	if _, err := parse("unknown"); err == nil {
		t.Fatal("got nil error, want unknown-value error")
	}
	defer func() {
		if recover() == nil {
			t.Error("got no panic, want unknown-value panic")
		}
	}()
	mustParse("unknown")
}
