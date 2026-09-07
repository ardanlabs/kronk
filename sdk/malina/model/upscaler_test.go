package model

import (
	"context"
	"errors"
	"image"
	"testing"
)

func TestNewUpscalerValidation(t *testing.T) {
	canceled, cancel := context.WithCancel(t.Context())
	cancel()

	tests := []struct {
		name    string
		ctx     context.Context
		cfg     UpscalerConfig
		wantErr error
	}{
		{name: "canceled context", ctx: canceled, cfg: UpscalerConfig{ModelPath: "model"}, wantErr: context.Canceled},
		{name: "missing model", ctx: t.Context(), cfg: UpscalerConfig{}},
		{name: "negative CPU threads", ctx: t.Context(), cfg: UpscalerConfig{ModelPath: "model", CPUThreads: -1}},
		{name: "negative tile size", ctx: t.Context(), cfg: UpscalerConfig{ModelPath: "model", TileSize: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewUpscaler(tt.ctx, tt.cfg)
			if err == nil {
				t.Fatal("NewUpscaler() error = nil, want error")
			}

			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Errorf("NewUpscaler() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestUpscalerWithoutNativeContext(t *testing.T) {
	cfg := UpscalerConfig{ModelPath: "model", CPUThreads: 4, TileSize: 128}
	u := Upscaler{config: cfg, unloaded: true}

	if got := u.Config(); got != cfg {
		t.Errorf("Config(): got %+v, want %+v", got, cfg)
	}

	if _, err := u.Factor(t.Context()); err == nil {
		t.Fatal("Factor() error = nil, want unloaded error")
	}

	input := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if _, err := u.Upscale(t.Context(), input); err == nil {
		t.Fatal("Upscale() error = nil, want unloaded error")
	}

	if err := u.Unload(); err != nil {
		t.Fatalf("Unload() error = %v", err)
	}

	if err := u.Unload(); err != nil {
		t.Fatalf("second Unload() error = %v", err)
	}
}

func TestUpscalerRequestValidation(t *testing.T) {
	u := Upscaler{}

	if _, err := u.Upscale(t.Context(), nil); !errors.Is(err, ErrInvalidRequest) {
		t.Errorf("Upscale() error = %v, want ErrInvalidRequest", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := u.Factor(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Factor() error = %v, want context.Canceled", err)
	}

	input := image.NewRGBA(image.Rect(0, 0, 8, 8))
	if _, err := u.Upscale(ctx, input); !errors.Is(err, context.Canceled) {
		t.Errorf("Upscale() error = %v, want context.Canceled", err)
	}
}
