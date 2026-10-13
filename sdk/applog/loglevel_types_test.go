package applog_test

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/ardanlabs/kronk/sdk/applog"
	"go.yaml.in/yaml/v2"
)

func TestLogLevel(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value applog.LogLevel
		want  int
	}{
		{"", applog.LogLevel{}, 0},
		{"silent", applog.LogSilent, 1},
		{"normal", applog.LogNormal, 2},
	} {
		t.Run(strconv.Itoa(tt.want), func(t *testing.T) {
			if tt.value.Int() != tt.want || tt.value.String() != tt.name || tt.value.IsZero() != (tt.want == 0) {
				t.Fatalf("got %q/%d, want %q/%d", tt.value, tt.value.Int(), tt.name, tt.want)
			}
			for _, input := range []string{tt.name, strconv.Itoa(tt.want)} {
				got, err := applog.ParseLogLevel(input)
				if err != nil || !got.Equal(tt.value) || !applog.MustParseLogLevel(input).Equal(tt.value) {
					t.Fatalf("parse %q: got %v/%v, want %v", input, got, err, tt.value)
				}
			}
			text, err := tt.value.MarshalText()
			if err != nil || string(text) != tt.name {
				t.Fatalf("text: got %q/%v, want %q", text, err, tt.name)
			}
			var fromText applog.LogLevel
			if err := fromText.UnmarshalText(text); err != nil || !fromText.Equal(tt.value) {
				t.Fatalf("text round trip: got %v/%v, want %v", fromText, err, tt.value)
			}
			for _, codec := range []struct {
				name      string
				marshal   func(any) ([]byte, error)
				unmarshal func([]byte, any) error
				want      string
			}{
				{"json", json.Marshal, json.Unmarshal, strconv.Itoa(tt.want)},
				{"yaml", yaml.Marshal, yaml.Unmarshal, strconv.Itoa(tt.want) + "\n"},
			} {
				data, err := codec.marshal(tt.value)
				if err != nil || string(data) != codec.want {
					t.Fatalf("%s: got %q/%v, want %q", codec.name, data, err, codec.want)
				}
				var got applog.LogLevel
				if err := codec.unmarshal(data, &got); err != nil || !got.Equal(tt.value) {
					t.Fatalf("%s round trip: got %v/%v, want %v", codec.name, got, err, tt.value)
				}
				got = applog.LogNormal
				if err := codec.unmarshal([]byte("99"), &got); err == nil || !got.Equal(applog.LogNormal) {
					t.Fatalf("%s invalid value: got %v/%v, want unchanged/error", codec.name, got, err)
				}
			}
		})
	}
	if _, err := applog.ParseLogLevel("unknown"); err == nil {
		t.Fatal("got nil error, want unknown-value error")
	}
	defer func() {
		if recover() == nil {
			t.Error("got no panic, want unknown-value panic")
		}
	}()
	applog.MustParseLogLevel("unknown")
}
