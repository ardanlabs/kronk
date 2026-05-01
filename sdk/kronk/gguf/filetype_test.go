package gguf

import "testing"

func TestFileTypeName(t *testing.T) {
	tests := []struct {
		ft   int64
		want string
	}{
		{0, "F32"},
		{1, "F16"},
		{7, "Q8_0"},
		{15, "Q4_K_M"},
		{17, "Q5_K_M"},
		{28, "BF16"},
		{999, "unknown(999)"},
	}

	for _, tt := range tests {
		got := FileTypeName(tt.ft)
		if got != tt.want {
			t.Errorf("FileTypeName(%d) = %q, want %q", tt.ft, got, tt.want)
		}
	}
}
