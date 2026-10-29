package source

import "testing"

func TestInputsFromMetadata(t *testing.T) {
	// The encoder flags of real projectors: Qwen2-Audio, Qwen2.5-VL and
	// Qwen2.5-Omni.
	for _, tt := range []struct {
		md   map[string]string
		want Inputs
		text string
	}{
		{map[string]string{"clip.has_audio_encoder": "true", "clip.projector_type": "qwen2a"}, Inputs{Known: true, Audio: true}, "audio"},
		{map[string]string{"clip.has_vision_encoder": "true"}, Inputs{Known: true, Images: true}, "images"},
		{map[string]string{"clip.has_vision_encoder": "true", "clip.has_audio_encoder": "true"}, Inputs{Known: true, Images: true, Audio: true}, "images and audio"},
		{map[string]string{"clip.projector_type": "mlp"}, Inputs{}, "images or audio"},
	} {
		got := inputsFromMetadata(tt.md)
		if got != tt.want {
			t.Errorf("%v: inputs = %+v, want %+v", tt.md, got, tt.want)
		}
		f := Facts{Inputs: got}
		f.Info.HasProjection = true
		if text := f.InputsText(); text != tt.text {
			t.Errorf("%v: text = %q, want %q", tt.md, text, tt.text)
		}
	}
}
