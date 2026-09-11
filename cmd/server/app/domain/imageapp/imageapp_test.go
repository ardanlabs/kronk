package imageapp

import "testing"

func TestGenerationRequestDefaults(t *testing.T) {
	req := generationRequest{Model: "sd-1.5", Prompt: "lighthouse"}

	params, err := req.params()
	if err != nil {
		t.Fatalf("params() error = %v", err)
	}
	if params.Width != 512 || params.Height != 512 {
		t.Errorf("dimensions: got %dx%d, want 512x512", params.Width, params.Height)
	}
	if params.Steps != 20 || params.CFGScale != 7 || params.Seed != -1 {
		t.Errorf("defaults: got steps=%d cfg-scale=%v seed=%d", params.Steps, params.CFGScale, params.Seed)
	}
}

func TestGenerationRequestOptions(t *testing.T) {
	steps := 30
	cfgScale := float32(4.5)
	seed := int64(42)
	req := generationRequest{
		Model:          "sd-1.5",
		Prompt:         "lighthouse",
		NegativePrompt: "fog",
		Size:           "768x512",
		N:              1,
		ResponseFormat: "b64_json",
		Steps:          &steps,
		CFGScale:       &cfgScale,
		Seed:           &seed,
	}

	params, err := req.params()
	if err != nil {
		t.Fatalf("params() error = %v", err)
	}
	if params.Width != 768 || params.Height != 512 || params.Steps != steps || params.CFGScale != cfgScale || params.Seed != seed {
		t.Errorf("params: got %+v", params)
	}
	if params.NegativePrompt != req.NegativePrompt {
		t.Errorf("NegativePrompt: got %q, want %q", params.NegativePrompt, req.NegativePrompt)
	}
}

func TestGenerationRequestRejectsUnsupportedValues(t *testing.T) {
	tests := []struct {
		name string
		req  generationRequest
	}{
		{name: "missing model", req: generationRequest{Prompt: "image"}},
		{name: "missing prompt", req: generationRequest{Model: "sd-1.5"}},
		{name: "multiple images", req: generationRequest{Model: "sd-1.5", Prompt: "image", N: 2}},
		{name: "url response", req: generationRequest{Model: "sd-1.5", Prompt: "image", ResponseFormat: "url"}},
		{name: "malformed size", req: generationRequest{Model: "sd-1.5", Prompt: "image", Size: "512"}},
		{name: "invalid dimensions", req: generationRequest{Model: "sd-1.5", Prompt: "image", Size: "100x100"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.req.params(); err == nil {
				t.Fatal("params() error = nil, want error")
			}
		})
	}
}
