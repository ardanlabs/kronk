package malina

import (
	"context"
	"errors"

	"github.com/ardanlabs/kronk/sdk/malina/model"
)

// UpscalerConfig controls loading a standalone ESRGAN upscaler.
type UpscalerConfig = model.UpscalerConfig

// Upscaler provides a concurrency-safe standalone ESRGAN upscaler.
type Upscaler = model.Upscaler

// NewUpscaler loads a standalone ESRGAN upscaler using ctx.
func NewUpscaler(ctx context.Context, cfg UpscalerConfig) (*Upscaler, error) {
	if !Initialized() {
		return nil, errors.New("new-upscaler: the Init() function has not been called")
	}
	return model.NewUpscaler(ctx, cfg)
}
