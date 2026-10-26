package modelprofile_test

import (
	"testing"

	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile"
	"github.com/ardanlabs/kronk/sdk/kronk/modelprofile/arch"
)

func TestResolveBatchInputMixing(t *testing.T) {
	tests := []struct {
		name         string
		architecture string
		want         modelprofile.BatchInputMixing
	}{
		{name: "Qwen 2 language model", architecture: "qwen2", want: modelprofile.BatchInputMixingSupported},
		{name: "Qwen 3 vision MoE", architecture: "qwen3vlmoe", want: modelprofile.BatchInputMixingSupported},
		{name: "Qwen 4 experimental", architecture: "qwen4exp", want: modelprofile.BatchInputMixingSupported},
		{name: "CogVLM excluded upstream", architecture: "cogvlm", want: modelprofile.BatchInputMixingUnsupported},
		{name: "DeepSeek 4 excluded upstream", architecture: "deepseek4", want: modelprofile.BatchInputMixingUnsupported},
		{name: "Granite Switch uses GGUF name", architecture: "graniteswitch", want: modelprofile.BatchInputMixingUnsupported},
		{name: "Eagle 3 excluded upstream", architecture: "eagle3", want: modelprofile.BatchInputMixingUnsupported},
		{name: "DFlash excluded upstream", architecture: "dflash", want: modelprofile.BatchInputMixingUnsupported},
		{name: "Gemma 4 assistant excluded upstream", architecture: "gemma4-assistant", want: modelprofile.BatchInputMixingUnsupported},
		{name: "unvalidated architecture", architecture: "future-model", want: modelprofile.BatchInputMixingUnknown},
		{name: "missing architecture", architecture: "", want: modelprofile.BatchInputMixingUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := arch.Resolve(map[string]string{"general.architecture": tt.architecture})
			if profile.Batch.InputMixing != tt.want {
				t.Errorf("input mixing = %q, want %q", profile.Batch.InputMixing, tt.want)
			}
		})
	}
}

func TestResolveVisionEncoderDoesNotEnableLanguageBatchCapabilities(t *testing.T) {
	profile := arch.Resolve(map[string]string{
		"general.architecture":     "qwen2vl",
		"clip.has_llava_projector": "true",
	})

	if profile.Role != modelprofile.RoleVisionEncoder {
		t.Fatalf("role = %q, want %q", profile.Role, modelprofile.RoleVisionEncoder)
	}
	if profile.Batch.InputMixing != modelprofile.BatchInputMixingUnknown {
		t.Errorf("input mixing = %q, want unknown", profile.Batch.InputMixing)
	}
}
