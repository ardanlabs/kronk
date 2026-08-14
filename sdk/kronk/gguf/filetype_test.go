package gguf

import "testing"

func TestFileTypeName(t *testing.T) {
	tests := []struct {
		ft   int64
		want string
	}{
		{0, "F32"},
		{1, "F16"},
		{2, "Q4_0"},
		{3, "Q4_1"},
		{7, "Q8_0"},
		{8, "Q5_0"},
		{9, "Q5_1"},
		{10, "Q2_K"},
		{11, "Q3_K_S"},
		{12, "Q3_K_M"},
		{13, "Q3_K_L"},
		{14, "Q4_K_S"},
		{15, "Q4_K_M"},
		{16, "Q5_K_S"},
		{17, "Q5_K_M"},
		{18, "Q6_K"},
		{19, "IQ2_XXS"},
		{20, "IQ2_XS"},
		{21, "Q2_K_S"},
		{22, "IQ3_XS"},
		{23, "IQ3_XXS"},
		{24, "IQ1_S"},
		{25, "IQ4_NL"},
		{26, "IQ3_S"},
		{27, "IQ3_M"},
		{28, "IQ2_S"},
		{29, "IQ2_M"},
		{30, "IQ4_XS"},
		{31, "IQ1_M"},
		{32, "BF16"},
		{36, "TQ1_0"},
		{37, "TQ2_0"},
		{38, "MXFP4_MOE"},
		{39, "NVFP4"},
		{40, "Q1_0"},
		{41, "Q2_0"},
	}

	if got, want := len(fileTypeNames), len(tests); got != want {
		t.Fatalf("fileTypeNames length: got %d, want %d", got, want)
	}

	for _, tt := range tests {
		got := FileTypeName(tt.ft)
		if got != tt.want {
			t.Errorf("FileTypeName(%d) = %q, want %q", tt.ft, got, tt.want)
		}
	}

	unknownTests := []struct {
		ft   int64
		want string
	}{
		{33, "unknown(33)"},
		{999, "unknown(999)"},
	}

	for _, tt := range unknownTests {
		if got := FileTypeName(tt.ft); got != tt.want {
			t.Errorf("FileTypeName(%d) = %q, want %q", tt.ft, got, tt.want)
		}
	}
}
