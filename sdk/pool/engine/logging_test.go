package engine_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/pool/engine"
)

func TestHumanBytes(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{name: "negative", input: -1, want: "-1B"},
		{name: "zero", input: 0, want: "0B"},
		{name: "decimal kilobyte", input: 1000, want: "1000B"},
		{name: "below kibibyte", input: 1023, want: "1023B"},
		{name: "kibibyte", input: 1 << 10, want: "1.0KiB"},
		{name: "mebibyte", input: 1 << 20, want: "1.0MiB"},
		{name: "fractional gibibyte", input: 3 << 29, want: "1.5GiB"},
		{name: "tebibyte", input: 1 << 40, want: "1.0TiB"},
		{name: "pebibyte", input: 1 << 50, want: "1.0PiB"},
		{name: "above pebibyte", input: 1 << 60, want: "1024.0PiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := engine.HumanBytes(tt.input); got != tt.want {
				t.Errorf("HumanBytes: got %q, want %q", got, tt.want)
			}
		})
	}
}
