package modelprofile

import "testing"

func TestSupportsTensorParallel(t *testing.T) {
	tests := []struct {
		name         string
		architecture string
		want         bool
	}{
		{"missing architecture", "", false},
		{"supported dense architecture", "llama", true},
		{"supported MoE architecture", "qwen3moe", true},
		{"unsupported recurrent architecture", "mamba", false},
		{"unsupported MoE architecture", "deepseek2", false},
		{"unsupported experimental architecture", "qwen4exp", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := supportsTensorParallel(tt.architecture); got != tt.want {
				t.Errorf("supportsTensorParallel() = %t, want %t", got, tt.want)
			}
		})
	}
}
