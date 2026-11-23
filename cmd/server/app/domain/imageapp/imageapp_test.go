package imageapp

import (
	"image"
	"testing"
)

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

func TestEditParams(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 130, 99))
	req := generationRequest{Model: "sd-1.5", Prompt: "watercolor"}

	params, err := editParams(req, source, 0.6)
	if err != nil {
		t.Fatalf("editParams() error = %v", err)
	}
	if params.Width != 128 || params.Height != 96 {
		t.Errorf("dimensions: got %dx%d, want 128x96", params.Width, params.Height)
	}
	if params.InitImage.Bounds().Dx() != 128 || params.InitImage.Bounds().Dy() != 96 {
		t.Errorf("init image dimensions: got %dx%d, want 128x96", params.InitImage.Bounds().Dx(), params.InitImage.Bounds().Dy())
	}
	if params.Strength != 0.6 {
		t.Errorf("Strength: got %v, want 0.6", params.Strength)
	}
}

func TestNewBundleParameters(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 139, 119))
	for _, tt := range []struct {
		model         string
		cfg           float32
		width, height int
		reference     bool
	}{
		{"moss-image-edit-hq", 7, 128, 112, false},
		{"qwen-image-2.1", 6, 128, 96, true},
		{"flux2-klein-base-4b", 4, 136, 112, true},
	} {
		t.Run(tt.model, func(t *testing.T) {
			req := generationRequest{Model: tt.model, Prompt: "make the sky blue"}
			params, err := editParams(req, source, 0.75)
			if err != nil {
				t.Fatal(err)
			}
			if params.Width != tt.width || params.Height != tt.height || params.CFGScale != tt.cfg || params.Steps != 20 {
				t.Fatalf("parameters: got %+v, want %dx%d, CFG %v and 20 steps", params, tt.width, tt.height, tt.cfg)
			}
			if tt.reference {
				if params.InitImage != nil || len(params.RefImages) != 1 || params.RefImages[0] != source {
					t.Fatalf("reference edit: got init %v and references %v, want source as reference only", params.InitImage, params.RefImages)
				}
			} else if params.InitImage == nil || len(params.RefImages) != 0 {
				t.Fatal("MOSS edit must supply its workflow input image")
			}
			if tt.model != "flux2-klein-base-4b" {
				req.Size = "136x112"
				if _, err := editParams(req, source, 0.75); err == nil {
					t.Fatal("unaligned explicit size accepted")
				}
			}
		})
	}
}

func TestEditParamsRejectsStrength(t *testing.T) {
	source := image.NewRGBA(image.Rect(0, 0, 128, 128))
	req := generationRequest{Model: "sd-1.5", Prompt: "watercolor"}

	if _, err := editParams(req, source, 0); err == nil {
		t.Fatal("editParams() error = nil, want error")
	}
}

func TestGenerationSizeScalesAndAligns(t *testing.T) {
	width, height, err := generationSize(image.Rect(0, 0, 1600, 901))
	if err != nil {
		t.Fatalf("generationSize() error = %v", err)
	}
	if width != 1024 || height != 576 {
		t.Errorf("generationSize(): got %dx%d, want 1024x576", width, height)
	}
}
