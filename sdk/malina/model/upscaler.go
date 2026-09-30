package model

import (
	"context"
	"errors"
	"fmt"
	"image"
	"strings"
	"sync"

	"github.com/ardanlabs/malina/pkg/sd"
)

// UpscalerConfig controls loading a standalone ESRGAN upscaler.
type UpscalerConfig struct {
	ModelPath     string
	Direct        bool
	CPUThreads    int32
	TileSize      int32
	Backend       string
	ParamsBackend string
}

// Upscaler owns one native ESRGAN context.
type Upscaler struct {
	mu       sync.Mutex
	config   UpscalerConfig
	ctx      sd.UpscalerContext
	factor   int32
	unloaded bool
}

// NewUpscaler loads a reusable ESRGAN model context.
func NewUpscaler(ctx context.Context, cfg UpscalerConfig) (*Upscaler, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.ModelPath) == "" {
		return nil, errors.New("new-upscaler: model path is required")
	}
	if cfg.CPUThreads < 0 {
		return nil, errors.New("new-upscaler: CPU threads cannot be negative")
	}
	if cfg.TileSize < 0 {
		return nil, errors.New("new-upscaler: tile size cannot be negative")
	}
	if cfg.CPUThreads == 0 {
		cfg.CPUThreads = sd.NumPhysicalCores()
	}

	var handle sd.UpscalerContext
	var factor int32
	err := withNative(ctx, func() error {
		var err error
		factor, err = sd.GetUpscalerModelScale(cfg.ModelPath)
		if err != nil {
			return fmt.Errorf("reading model scale: %w", err)
		}
		if factor < 1 {
			return errors.New("model is not a recognized RGB ESRGAN upscaler")
		}

		handle, err = sd.NewUpscalerContext(cfg.ModelPath, cfg.Direct, cfg.CPUThreads, cfg.TileSize, cfg.Backend, cfg.ParamsBackend)
		if err != nil {
			return fmt.Errorf("creating context: %w", err)
		}

		loadedFactor, err := sd.GetUpscaleFactor(handle)
		if err != nil {
			sd.FreeUpscalerContext(handle)
			handle = 0
			return fmt.Errorf("reading loaded context scale: %w", err)
		}
		if loadedFactor != factor {
			sd.FreeUpscalerContext(handle)
			handle = 0
			return fmt.Errorf("model metadata scale %d differs from loaded context scale %d", factor, loadedFactor)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("new-upscaler: %w", err)
	}

	u := Upscaler{config: cfg, ctx: handle, factor: factor}
	return &u, nil
}

// Config returns immutable upscaler configuration.
func (u *Upscaler) Config() UpscalerConfig {
	return u.config
}

// Factor returns the native scale read from the model's metadata during
// construction.
func (u *Upscaler) Factor(ctx context.Context) (int, error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.unloaded {
		return 0, errors.New("upscale factor: model is unloaded")
	}

	if err := ctx.Err(); err != nil {
		return 0, err
	}

	return int(u.factor), nil
}

// Upscale enlarges an image using the model's native scale factor.
// Cancellation cannot interrupt an ESRGAN call that has already entered native code.
func (u *Upscaler) Upscale(ctx context.Context, input image.Image) ([]image.Image, error) {
	if input == nil {
		return nil, errors.Join(ErrInvalidRequest, errors.New("upscale image is required"))
	}

	raw, err := imageToRGB(input)
	if err != nil {
		return nil, err
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	if u.unloaded {
		return nil, errors.New("upscale: model is unloaded")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var output []*sd.SDImage
	err = withGeneration(ctx, context.Background(), func() error {
		output, err = sd.Upscale(u.ctx, raw, uint32(u.factor))
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("upscale: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	images := make([]image.Image, len(output))
	for i, result := range output {
		images[i], err = decodeImage(result)
		if err != nil {
			return nil, fmt.Errorf("upscale: decode image %d: %w", i, err)
		}
	}

	return images, nil
}

// Unload releases the native upscaler context exactly once.
func (u *Upscaler) Unload() error {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.unloaded {
		return nil
	}

	if err := withNative(context.Background(), func() error {
		sd.FreeUpscalerContext(u.ctx)
		return nil
	}); err != nil {
		return fmt.Errorf("unload upscaler: %w", err)
	}

	u.ctx = 0
	u.factor = 0
	u.unloaded = true

	return nil
}
