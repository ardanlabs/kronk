// Package model provides the low-level API for working with
// whisper.cpp models via the github.com/ardanlabs/bucky FFI
// bindings. It owns the whisper.Context lifecycle, parameter
// translation, and the transcribe / language-detect primitives the
// high-level sdk/bucky package layers concurrency on top of.
package model

import (
	"github.com/ardanlabs/kronk/sdk/applog"
)

// Config carries the per-model whisper.cpp configuration. Fields are
// resolved through the functional Option pattern (NewConfig +
// WithX) at construction time and treated as read-only thereafter.
//
// ModelPath is required. The remaining fields all have sensible zero
// defaults that match whisper_context_default_params and the
// per-handle backpressure conventions used by sdk/kronk.
type Config struct {
	// ModelPath is the absolute path to the GGML whisper model file
	// the handle will load via whisper.InitFromFileWithParams.
	ModelPath string

	// UseGPU enables GPU offload (Metal on darwin, CUDA / Vulkan on
	// linux+windows when libwhisper was built with the relevant
	// backend). Defaults to whisper.cpp's own default (true).
	UseGPU bool

	// FlashAttn enables the flash-attention kernel when supported by
	// the active backend. Defaults to false.
	FlashAttn bool

	// GPUDevice selects which GPU the model is offloaded to when
	// multiple devices are present. Defaults to 0.
	GPUDevice int32

	// NThreads is the default thread count attached to every
	// Transcribe call when no per-call override is supplied. A zero
	// value means whisper.cpp's own default (typically min(4, ncpu)).
	NThreads int32

	// NSeqMax bounds the number of concurrent transcribe sequences
	// allowed against a single handle. The whisper context itself is
	// single-stream so this is effectively the depth of the
	// per-handle semaphore. Values <= 0 collapse to 1.
	NSeqMax int

	// QueueDepth multiplies NSeqMax to give the effective semaphore
	// capacity, matching the kronk discipline. Values <= 0 collapse
	// to 2.
	QueueDepth int

	// Log is the logger the model uses for diagnostic output.
	// Defaults to applog.DiscardLogger when nil.
	Log applog.Logger
}

// WithDefaults returns cfg with the zero-valued fields filled in.
func (cfg Config) WithDefaults() Config {
	if cfg.NSeqMax <= 0 {
		cfg.NSeqMax = 1
	}
	if cfg.QueueDepth <= 0 {
		cfg.QueueDepth = 2
	}
	if cfg.Log == nil {
		cfg.Log = applog.DiscardLogger
	}
	return cfg
}

// =============================================================================

// Option represents a functional option for configuring a Config.
type Option func(*Config)

// NewConfig builds a Config from the supplied options. Zero-valued
// fields are left as zero — defaulting happens inside Model
// construction via Config.WithDefaults.
func NewConfig(opts ...Option) Config {
	var cfg Config
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}

// WithConfig replaces the entire Config in one shot. Useful for pool
// loaders that resolve a fully populated Config from a catalog.
func WithConfig(src Config) Option { return func(c *Config) { *c = src } }

// WithModelPath sets the GGML model file the handle will load.
func WithModelPath(v string) Option { return func(c *Config) { c.ModelPath = v } }

// WithUseGPU toggles GPU offload at model-load time.
func WithUseGPU(v bool) Option { return func(c *Config) { c.UseGPU = v } }

// WithFlashAttn toggles the flash-attention kernel.
func WithFlashAttn(v bool) Option { return func(c *Config) { c.FlashAttn = v } }

// WithGPUDevice selects a specific GPU device index.
func WithGPUDevice(v int32) Option { return func(c *Config) { c.GPUDevice = v } }

// WithNThreads sets the default thread count for Transcribe.
func WithNThreads(v int32) Option { return func(c *Config) { c.NThreads = v } }

// WithNSeqMax sets the per-handle max concurrent sequence count.
func WithNSeqMax(v int) Option { return func(c *Config) { c.NSeqMax = v } }

// WithQueueDepth multiplies NSeqMax to size the per-handle semaphore.
func WithQueueDepth(v int) Option { return func(c *Config) { c.QueueDepth = v } }

// WithLog sets the logger the model and its operations use.
func WithLog(v applog.Logger) Option { return func(c *Config) { c.Log = v } }
