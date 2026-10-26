package modelprofile_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
)

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
		{"unsupported GLM5 Next architecture", "glm5-next", false},
		{"unsupported experimental architecture", "qwen4exp", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arch.Resolve(map[string]string{"general.architecture": tt.architecture}).SupportsTensorParallel; got != tt.want {
				t.Errorf("SupportsTensorParallel = %t, want %t", got, tt.want)
			}
		})
	}
}
