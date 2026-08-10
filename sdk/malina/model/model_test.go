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
		{name: "invalid queue", opts: []Option{WithModelPath("model"), WithQueueDepth(0)}, wantErr: true},
		{name: "invalid timeout", opts: []Option{WithModelPath("model"), WithAdmissionTimeout(-time.Second)}, wantErr: true},
		{name: "invalid threads", opts: []Option{WithModelPath("model"), WithCPUThreads(-1)}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := NewConfig(tt.opts...)
			if (err != nil) != tt.wantErr {
				t.Fatalf("NewConfig() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && (cfg.QueueDepth != defaultQueueDepth || cfg.AdmissionTimeout != defaultAdmissionTimeout) {
				t.Errorf("defaults: got %d/%s, want %d/%s", cfg.QueueDepth, cfg.AdmissionTimeout, defaultQueueDepth, defaultAdmissionTimeout)
			}
		})
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

func TestWithNativeStoppedWaitDoesNotRun(t *testing.T) {
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
		result <- withNativeContexts(t.Context(), stop, func() error {
			called.Store(true)
			return nil
		})
	}()
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("withNativeContexts() error = %v, want context.Canceled", err)
	}
	if called.Load() {
		t.Fatal("withNativeContexts() ran stopped callback")
	}

	close(release)
	if err := <-first; err != nil {
		t.Fatalf("first withNative() error = %v", err)
	}
}
