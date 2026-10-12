package models_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/tools/models"
)

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		name  string
		input int64
		want  string
	}{
		{name: "negative", input: -1, want: ""},
		{name: "zero", input: 0, want: ""},
		{name: "bytes", input: 1023, want: "1023 B"},
		{name: "kibibyte", input: 1 << 10, want: "1 KiB"},
		{name: "mebibyte", input: 1 << 20, want: "1 MiB"},
		{name: "gibibyte", input: 1 << 30, want: "1.00 GiB"},
		{name: "fractional gibibyte", input: 3 << 29, want: "1.50 GiB"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := models.FormatBytes(tt.input); got != tt.want {
				t.Errorf("FormatBytes: got %q, want %q", got, tt.want)
			}
		})
	}
}
