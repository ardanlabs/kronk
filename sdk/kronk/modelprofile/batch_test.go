package modelprofile

import "testing"

func TestResolveBatchInputMixing(t *testing.T) {
	tests := []struct {
		name         string
		architecture string
		want         BatchInputMixing
	}{
		{name: "Qwen 2 language model", architecture: "qwen2", want: BatchInputMixingSupported},
		{name: "Qwen 3 vision MoE", architecture: "qwen3vlmoe", want: BatchInputMixingSupported},
		{name: "Qwen 4 experimental", architecture: "qwen4exp", want: BatchInputMixingSupported},
		{name: "CogVLM excluded upstream", architecture: "cogvlm", want: BatchInputMixingUnsupported},
		{name: "DeepSeek 4 excluded upstream", architecture: "deepseek4", want: BatchInputMixingUnsupported},
		{name: "Granite Switch uses GGUF name", architecture: "graniteswitch", want: BatchInputMixingUnsupported},
		{name: "Eagle 3 excluded upstream", architecture: "eagle3", want: BatchInputMixingUnsupported},
		{name: "DFlash excluded upstream", architecture: "dflash", want: BatchInputMixingUnsupported},
		{name: "Gemma 4 assistant excluded upstream", architecture: "gemma4-assistant", want: BatchInputMixingUnsupported},
		{name: "unvalidated architecture", architecture: "future-model", want: BatchInputMixingUnknown},
		{name: "missing architecture", architecture: "", want: BatchInputMixingUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := Resolve(map[string]string{"general.architecture": tt.architecture})
			if profile.Batch.InputMixing != tt.want {
				t.Errorf("input mixing = %q, want %q", profile.Batch.InputMixing, tt.want)
			}
		})
	}
}

func TestResolveVisionEncoderDoesNotEnableLanguageBatchCapabilities(t *testing.T) {
	profile := Resolve(map[string]string{
		"general.architecture":     "qwen2vl",
		"clip.has_llava_projector": "true",
	})
	if profile.Role != RoleVisionEncoder {
		t.Fatalf("role = %q, want %q", profile.Role, RoleVisionEncoder)
	}
	if profile.Batch.InputMixing != BatchInputMixingUnknown {
		t.Errorf("input mixing = %q, want unknown", profile.Batch.InputMixing)
	}
}
