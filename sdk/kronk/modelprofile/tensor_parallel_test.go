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
		{"supported GLM5 Next architecture", "glm5-next", true},
		{"mixed-case GLM5 Next architecture", "GLM5-NEXT", true},
		{"supported GLM4 MoE architecture", "glm4moe", true},
		{"unsupported GLM DSA architecture", "glm-dsa", false},
		{"unsupported experimental architecture", "qwen4exp", false},
		{"unsupported Kimi K3.1 architecture", "kimi-k31", false},
		{"mixed-case Kimi K3.1 architecture", "KIMI-K31", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := arch.Resolve(map[string]string{"general.architecture": tt.architecture}).SupportsTensorParallel; got != tt.want {
				t.Errorf("SupportsTensorParallel = %t, want %t", got, tt.want)
			}
		})
	}
}
