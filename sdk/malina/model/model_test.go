package model

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ardanlabs/malina/pkg/sd"
)

func TestNewConfig(t *testing.T) {
	tests := []struct {
		name    string
		opts    []Option
		wantErr bool
	}{
		{name: "checkpoint", opts: []Option{WithModelPath("model")}},
		{name: "component", opts: []Option{WithDiffusionModelPath("diffusion"), WithVAEPath("vae"), WithLLMPath("llm")}},
		{name: "no model", wantErr: true},
		{name: "zero queue", opts: []Option{WithModelPath("model"), WithQueueDepth(0)}},
		{name: "invalid concurrency", opts: []Option{WithModelPath("model"), WithConcurrency(0)}, wantErr: true},
		{name: "invalid queue", opts: []Option{WithModelPath("model"), WithQueueDepth(-1)}, wantErr: true},
		{name: "invalid timeout", opts: []Option{WithModelPath("model"), WithAdmissionTimeout(-time.Second)}, wantErr: true},
		{name: "invalid threads", opts: []Option{WithModelPath("model"), WithCPUThreads(-1)}, wantErr: true},
		{name: "linear scale", opts: []Option{WithModelPath("model"), WithLinearScale(0.125)}},
		{name: "negative linear scale", opts: []Option{WithModelPath("model"), WithLinearScale(-1)}, wantErr: true},
		{name: "infinite linear scale", opts: []Option{WithModelPath("model"), WithLinearScale(float32(math.Inf(1)))}, wantErr: true},
		{name: "attention scale", opts: []Option{WithModelPath("model"), WithAttnScale(0.25)}},
		{name: "negative attention scale", opts: []Option{WithModelPath("model"), WithAttnScale(-1)}, wantErr: true},
		{name: "NaN attention scale", opts: []Option{WithModelPath("model"), WithAttnScale(float32(math.NaN()))}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewConfig(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewConfigDefaults(t *testing.T) {
	cfg, err := NewConfig(WithModelPath("model"))
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if cfg.Concurrency != defaultConcurrency || cfg.QueueDepth != defaultQueueDepth || cfg.AdmissionTimeout != defaultAdmissionTimeout {
		t.Errorf("defaults: got %d/%d/%s, want %d/%d/%s", cfg.Concurrency, cfg.QueueDepth, cfg.AdmissionTimeout, defaultConcurrency, defaultQueueDepth, defaultAdmissionTimeout)
	}
	if cfg.LinearScale != 0 || cfg.AttnScale != 0 {
		t.Errorf("scale defaults: got %g/%g, want 0/0", cfg.LinearScale, cfg.AttnScale)
	}
}

func TestScaleOptions(t *testing.T) {
	cfg, err := NewConfig(
		WithModelPath("model"),
		WithLinearScale(0.125),
		WithAttnScale(0.25),
	)
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	if cfg.LinearScale != 0.125 || cfg.AttnScale != 0.25 {
		t.Errorf("scales: got %g/%g, want 0.125/0.25", cfg.LinearScale, cfg.AttnScale)
	}
}

func TestGenerateParamsValidate(t *testing.T) {
	valid := NewGenerateParams()
	valid.Prompt = "cat"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*GenerateParams)
	}{
		{name: "missing prompt", mutate: func(p *GenerateParams) { p.Prompt = "" }},
		{name: "NUL prompt", mutate: func(p *GenerateParams) { p.Prompt = "cat\x00dog" }},
		{name: "invalid width", mutate: func(p *GenerateParams) { p.Width = 63 }},
		{name: "invalid steps", mutate: func(p *GenerateParams) { p.Steps = 0 }},
		{name: "invalid CFG", mutate: func(p *GenerateParams) { p.CFGScale = float32(math.NaN()) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := valid
			tt.mutate(&params)
			if err := params.Validate(); !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Validate() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestWorkflowConfigOptions(t *testing.T) {
	cfg, err := NewConfig(
		WithModelPath("model"),
		WithControlNetPath("controlnet"),
		WithMotionModulePath("motion"),
		WithADetailerPath("adetailer"),
	)
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}

	if cfg.ControlNetPath != "controlnet" || cfg.MotionModulePath != "motion" || cfg.ADetailerPath != "adetailer" {
		t.Errorf("workflow paths: got %q/%q/%q, want controlnet/motion/adetailer", cfg.ControlNetPath, cfg.MotionModulePath, cfg.ADetailerPath)
	}

	mdl := Model{config: cfg, version: "Stable Diffusion 1.x"}
	if got := mdl.Config(); got != cfg {
		t.Errorf("Config(): got %+v, want %+v", got, cfg)
	}

	info := mdl.Info()
	if info.MotionModulePath != "motion" || info.ADetailerPath != "adetailer" || info.ModelVersion != "Stable Diffusion 1.x" {
		t.Errorf("Info(): got motion=%q adetailer=%q version=%q, want motion/adetailer/Stable Diffusion 1.x", info.MotionModulePath, info.ADetailerPath, info.ModelVersion)
	}
}

func TestCannyParams(t *testing.T) {
	got := NewCannyParams()
	want := CannyParams{HighThreshold: 0.08, LowThreshold: 0.08, Weak: 0.8, Strong: 1}
	if got != want {
		t.Errorf("NewCannyParams(): got %+v, want %+v", got, want)
	}

	valid := NewGenerateParams()
	valid.Prompt = "cat"
	valid.ControlImage = image.NewRGBA(image.Rect(0, 0, 64, 64))
	valid.Canny = &got
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*GenerateParams)
	}{
		{name: "zero control strength", mutate: func(p *GenerateParams) { p.ControlStrength = 0 }},
		{name: "NaN control strength", mutate: func(p *GenerateParams) { p.ControlStrength = float32(math.NaN()) }},
		{name: "high threshold", mutate: func(p *GenerateParams) { p.Canny.HighThreshold = 1.1 }},
		{name: "low threshold", mutate: func(p *GenerateParams) { p.Canny.LowThreshold = -0.1 }},
		{name: "weak", mutate: func(p *GenerateParams) { p.Canny.Weak = float32(math.NaN()) }},
		{name: "strong", mutate: func(p *GenerateParams) { p.Canny.Strong = float32(math.Inf(1)) }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := NewGenerateParams()
			params.Prompt = "cat"
			params.ControlImage = image.NewRGBA(image.Rect(0, 0, 64, 64))
			canny := NewCannyParams()
			params.Canny = &canny
			tt.mutate(&params)

			if err := params.Validate(); !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Validate() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestDetailParams(t *testing.T) {
	got := NewDetailParams()
	if got.ExtraArgs != "input_size=640,confidence=0.3,inpaint_width=64,inpaint_height=64" || got.Steps != 20 || got.CFGScale != 7 || got.Seed != -1 {
		t.Errorf("NewDetailParams(): got %+v, want default detail parameters", got)
	}

	valid := NewDetailParams()
	valid.Image = image.NewRGBA(image.Rect(0, 0, 64, 64))
	valid.Prompt = "portrait"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*DetailParams)
	}{
		{name: "missing image", mutate: func(p *DetailParams) { p.Image = nil }},
		{name: "invalid width", mutate: func(p *DetailParams) { p.Image = image.NewRGBA(image.Rect(0, 0, 65, 64)) }},
		{name: "invalid height", mutate: func(p *DetailParams) { p.Image = image.NewRGBA(image.Rect(0, 0, 64, 65)) }},
		{name: "missing prompt", mutate: func(p *DetailParams) { p.Prompt = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := NewDetailParams()
			params.Image = image.NewRGBA(image.Rect(0, 0, 64, 64))
			params.Prompt = "portrait"
			tt.mutate(&params)

			if err := params.Validate(); !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Validate() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestVideoParams(t *testing.T) {
	got := NewVideoParams()
	want := VideoParams{Width: 128, Height: 128, Steps: 4, Seed: -1, Frames: 4, FPS: 1}
	if got != want {
		t.Errorf("NewVideoParams(): got %+v, want %+v", got, want)
	}

	valid := NewVideoParams()
	valid.Prompt = "walking cat"
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*VideoParams)
	}{
		{name: "zero frames", mutate: func(p *VideoParams) { p.Frames = 0 }},
		{name: "too many frames", mutate: func(p *VideoParams) { p.Frames = 1_001 }},
		{name: "zero FPS", mutate: func(p *VideoParams) { p.FPS = 0 }},
		{name: "too much FPS", mutate: func(p *VideoParams) { p.FPS = 1_001 }},
		{name: "missing prompt", mutate: func(p *VideoParams) { p.Prompt = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := NewVideoParams()
			params.Prompt = "walking cat"
			tt.mutate(&params)

			if err := params.Validate(); !errors.Is(err, ErrInvalidRequest) {
				t.Errorf("Validate() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

func TestInitImageValidationAndConversion(t *testing.T) {
	source := image.NewRGBA(image.Rect(3, 4, 5, 5))
	source.Set(3, 4, color.RGBA{R: 10, G: 20, B: 30, A: 255})

	params := NewGenerateParams()
	params.Prompt = "cat"
	params.Width = 64
	params.Height = 64
	params.InitImage = source
	params.Strength = 0.5
	if err := params.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	raw, err := imageToRGB(source)
	if err != nil {
		t.Fatalf("imageToRGB() error = %v", err)
	}
	if raw.Width != 2 || raw.Height != 1 || len(raw.Data) != 6 || raw.Data[0] != 10 || raw.Data[1] != 20 || raw.Data[2] != 30 {
		t.Errorf("imageToRGB() = %+v, want 2x1 RGB beginning 10,20,30", raw)
	}

	invalid := []GenerateParams{params, params, params}
	invalid[0].Strength = 0
	invalid[1].Strength = 1.1
	invalid[2].Strength = float32(math.NaN())
	for i, p := range invalid {
		if err := p.Validate(); !errors.Is(err, ErrInvalidRequest) {
			t.Errorf("Validate() case %d error = %v, want ErrInvalidRequest", i, err)
		}
	}

	tooLarge := image.NewUniform(color.Black)
	params.InitImage = boundedImage{Image: tooLarge, bounds: image.Rect(0, 0, maxImageDimension+1, 1)}
	if err := params.Validate(); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Validate() large image error = %v, want ErrInvalidRequest", err)
	}
	if _, err := imageToRGB(params.InitImage); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("imageToRGB() large image error = %v, want ErrInvalidRequest", err)
	}
}

type boundedImage struct {
	image.Image
	bounds image.Rectangle
}

func (bi boundedImage) Bounds() image.Rectangle {
	return bi.bounds
}

func TestEncodeImage(t *testing.T) {
	raw := sd.SDImage{
		Width:   2,
		Height:  1,
		Channel: 3,
		Data:    []byte{255, 0, 0, 0, 255, 0},
	}

	got, err := encodeImage(&raw, 42)
	if err != nil {
		t.Fatalf("encodeImage() error = %v", err)
	}
	if _, err := png.Decode(bytes.NewReader(got.PNG)); err != nil {
		t.Fatalf("png.Decode() error = %v", err)
	}
	if got.Width != 2 || got.Height != 1 || got.Seed != 42 {
		t.Errorf("metadata: got %+v, want width 2, height 1, seed 42", got)
	}
}

func TestWithNativeCanceledWaitDoesNotRun(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- withNative(t.Context(), func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var called atomic.Bool
	if err := withNative(ctx, func() error {
		called.Store(true)
		return nil
	}); !errors.Is(err, context.Canceled) {
		t.Fatalf("withNative() error = %v, want context.Canceled", err)
	}
	if called.Load() {
		t.Fatal("withNative() ran canceled callback")
	}

	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first withNative() error = %v", err)
	}
}

func TestWithGenerationStoppedWaitDoesNotRun(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	first := make(chan error, 1)
	go func() {
		first <- withNative(t.Context(), func() error {
			close(entered)
			<-release
			return nil
		})
	}()
	<-entered

	stop, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	var called atomic.Bool
	go func() {
		result <- withGeneration(t.Context(), stop, func() error {
			called.Store(true)
			return nil
		})
	}()
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("withGeneration() error = %v, want context.Canceled", err)
	}
	if called.Load() {
		t.Fatal("withGeneration() ran stopped callback")
	}

	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first withNative() error = %v", err)
	}
}
