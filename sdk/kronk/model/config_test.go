package model

import (
	"testing"
)

func TestGGMLTypeString(t *testing.T) {
	tests := []struct {
		typ  GGMLType
		want string
	}{
		{GGMLTypeF32, "f32"},
		{GGMLTypeF16, "f16"},
		{GGMLTypeQ4_0, "q4_0"},
		{GGMLTypeQ4_1, "q4_1"},
		{GGMLTypeQ5_0, "q5_0"},
		{GGMLTypeQ5_1, "q5_1"},
		{GGMLTypeQ8_0, "q8_0"},
		{GGMLTypeBF16, "bf16"},
		{GGMLTypeAuto, "auto"},
		{GGMLType(999), "unknown(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.typ.String(); got != tt.want {
				t.Errorf("GGMLType.String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseGGMLType(t *testing.T) {
	tests := []struct {
		input   string
		want    GGMLType
		wantErr bool
	}{
		{"f32", GGMLTypeF32, false},
		{"fp32", GGMLTypeF32, false},
		{"F32", GGMLTypeF32, false},
		{"f16", GGMLTypeF16, false},
		{"fp16", GGMLTypeF16, false},
		{"F16", GGMLTypeF16, false},
		{"q4_0", GGMLTypeQ4_0, false},
		{"q4", GGMLTypeQ4_0, false},
		{"Q4_0", GGMLTypeQ4_0, false},
		{"q4_1", GGMLTypeQ4_1, false},
		{"q5_0", GGMLTypeQ5_0, false},
		{"q5", GGMLTypeQ5_0, false},
		{"q5_1", GGMLTypeQ5_1, false},
		{"q8_0", GGMLTypeQ8_0, false},
		{"q8", GGMLTypeQ8_0, false},
		{"Q8_0", GGMLTypeQ8_0, false},
		{"bf16", GGMLTypeBF16, false},
		{"bfloat16", GGMLTypeBF16, false},
		{"BF16", GGMLTypeBF16, false},
		{"auto", GGMLTypeAuto, false},
		{"", GGMLTypeAuto, false},
		{"  auto  ", GGMLTypeAuto, false},
		{"invalid", GGMLTypeAuto, true},
		{"q3", GGMLTypeAuto, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseGGMLType(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGGMLType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("ParseGGMLType() = %v, want %v", got, tt.want)
			}
		})
	}
}
