// Package model configures and owns reusable stable-diffusion model contexts
// for the Malina SDK.
//
// Experimental: This package's public API is subject to change.
package model

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/ardanlabs/malina/pkg/sd"
	"golang.org/x/sync/semaphore"
)

var (
	// ErrInvalidRequest identifies invalid generation parameters.
	ErrInvalidRequest = errors.New("invalid generation request")

	// ErrNativeGeneration identifies a failure returned by stable-diffusion.
	ErrNativeGeneration = errors.New("native generation failed")
)

const (
	defaultConcurrency      = 1
	defaultQueueDepth       = 0
	defaultAdmissionTimeout = 3 * time.Minute
	maxImageDimension       = 1024
	maxImagePixels          = maxImageDimension * maxImageDimension
)

// Config controls model loading and request admission. Concurrency controls
// the number of independently loaded contexts and simultaneous generations.
// QueueDepth controls how many calls are admitted to wait after every context
// is busy.
// ModelPath loads an all-in-one checkpoint. DiffusionModelPath and its
// companion paths configure a component model. At least one of ModelPath or
// DiffusionModelPath is required.
type Config struct {
	ModelPath                   string
	ClipLPath                   string
	ClipGPath                   string
	ClipVisionPath              string
	T5XXLPath                   string
	LLMPath                     string
	LLMVisionPath               string
	DiffusionModelPath          string
	HighNoiseDiffusionModelPath string
	EmbeddingsConnectorsPath    string
	VAEPath                     string
	AudioVAEPath                string
	TAESDPath                   string
	ControlNetPath              string
	PhotoMakerPath              string
	TensorTypeRules             string
	Concurrency                 int
	QueueDepth                  int
	AdmissionTimeout            time.Duration
	CPUThreads                  int32
}

// Option modifies Config.
type Option func(*Config)

// WithConfig replaces the model configuration.
func WithConfig(config Config) Option {
	return func(cfg *Config) {
		*cfg = config
	}
}

// WithModelPath sets an all-in-one model checkpoint path.
func WithModelPath(path string) Option {
	return func(cfg *Config) {
		cfg.ModelPath = path
	}
}

// WithDiffusionModelPath sets a component diffusion model path.
func WithDiffusionModelPath(path string) Option {
	return func(cfg *Config) {
		cfg.DiffusionModelPath = path
	}
}

// WithVAEPath sets a component VAE path.
func WithVAEPath(path string) Option {
	return func(cfg *Config) {
		cfg.VAEPath = path
	}
}

// WithLLMPath sets a component LLM text encoder path.
func WithLLMPath(path string) Option {
	return func(cfg *Config) {
		cfg.LLMPath = path
	}
}

// WithConcurrency sets the number of independently loaded model contexts and
// simultaneous generations.
func WithConcurrency(concurrency int) Option {
	return func(cfg *Config) {
		cfg.Concurrency = concurrency
	}
}

// WithQueueDepth sets the number of generation calls admitted to wait after all
// model contexts are busy.
func WithQueueDepth(depth int) Option {
	return func(cfg *Config) {
		cfg.QueueDepth = depth
	}
}

// WithAdmissionTimeout sets the maximum admission wait.
func WithAdmissionTimeout(timeout time.Duration) Option {
	return func(cfg *Config) {
		cfg.AdmissionTimeout = timeout
	}
}

// WithCPUThreads sets the number of native CPU worker threads.
func WithCPUThreads(threads int32) Option {
	return func(cfg *Config) {
		cfg.CPUThreads = threads
	}
}

// NewConfig constructs and validates Config.
func NewConfig(opts ...Option) (Config, error) {
	cfg := Config{
		Concurrency:      defaultConcurrency,
		QueueDepth:       defaultQueueDepth,
		AdmissionTimeout: defaultAdmissionTimeout,
	}
	for _, opt := range opts {
		opt(&cfg)
	}

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func validateConfig(cfg Config) error {
	if strings.TrimSpace(cfg.ModelPath) == "" && strings.TrimSpace(cfg.DiffusionModelPath) == "" {
		return errors.New("model configuration requires a model or diffusion model path")
	}
	if cfg.Concurrency < 1 {
		return errors.New("concurrency must be positive")
	}
	if cfg.QueueDepth < 0 {
		return errors.New("queue depth cannot be negative")
	}
	if cfg.AdmissionTimeout <= 0 {
		return errors.New("admission timeout must be positive")
	}
	if cfg.CPUThreads < 0 {
		return errors.New("CPU threads cannot be negative")
	}

	return nil
}

// GenerateParams controls one text-to-image generation.
type GenerateParams struct {
	Prompt         string
	NegativePrompt string
	Width          int
	Height         int
	Steps          int
	CFGScale       float32
	Seed           int64
	InitImage      image.Image
	Strength       float32
}

// NewGenerateParams returns stable-diffusion.cpp generation defaults.
func NewGenerateParams() GenerateParams {
	return GenerateParams{
		Width:    512,
		Height:   512,
		Steps:    20,
		CFGScale: 7,
		Seed:     -1,
		Strength: 0.75,
	}
}

// Validate checks whether parameters describe a supported image-generation
// request.
func (p GenerateParams) Validate() error {
	if strings.TrimSpace(p.Prompt) == "" {
		return errors.Join(ErrInvalidRequest, errors.New("prompt is required"))
	}
	if strings.IndexByte(p.Prompt, 0) >= 0 || strings.IndexByte(p.NegativePrompt, 0) >= 0 {
		return errors.Join(ErrInvalidRequest, errors.New("prompts cannot contain NUL bytes"))
	}
	if p.Width < 64 || p.Width > maxImageDimension || p.Height < 64 || p.Height > maxImageDimension || p.Width%8 != 0 || p.Height%8 != 0 || p.Width*p.Height > maxImagePixels {
		return errors.Join(ErrInvalidRequest, fmt.Errorf("dimensions must be multiples of 8 between 64 and %d and at most %d pixels", maxImageDimension, maxImagePixels))
	}
	if p.Steps < 1 || p.Steps > 1000 {
		return errors.Join(ErrInvalidRequest, errors.New("steps must be between 1 and 1000"))
	}
	if p.CFGScale <= 0 || math.IsNaN(float64(p.CFGScale)) || math.IsInf(float64(p.CFGScale), 0) {
		return errors.Join(ErrInvalidRequest, errors.New("CFG scale must be positive and finite"))
	}
	if p.InitImage != nil {
		if err := validateInitImage(p.InitImage); err != nil {
			return err
		}
		if p.Strength <= 0 || p.Strength > 1 || math.IsNaN(float64(p.Strength)) || math.IsInf(float64(p.Strength), 0) {
			return errors.Join(ErrInvalidRequest, errors.New("img2img strength must be finite and in (0,1]"))
		}
	}

	return nil
}

// GeneratedImage contains an owned PNG and generation metadata.
type GeneratedImage struct {
	PNG    []byte
	Width  int
	Height int
	Seed   int64
}

// ModelInfo describes a loaded model.
type ModelInfo struct {
	ModelPath          string
	DiffusionModelPath string
	CPUThreads         int32
}

// Model owns exactly one native stable-diffusion context.
type Model struct {
	mu       sync.Mutex
	config   Config
	ctx      sd.Context
	stop     context.Context
	cancel   context.CancelFunc
	unloaded bool
}

// Native stable-diffusion callbacks and diagnostics are process-global. The
// weighted gate gives generation shared access while keeping context
// construction and destruction exclusive from all other native operations.
// Each Model's mutex separately prevents concurrent use of one native context.
var nativeGate = semaphore.NewWeighted(math.MaxInt64)

func withNative(ctx context.Context, run func() error) error {
	if err := nativeGate.Acquire(ctx, math.MaxInt64); err != nil {
		return err
	}
	defer nativeGate.Release(math.MaxInt64)

	if err := ctx.Err(); err != nil {
		return err
	}

	return run()
}

func withGeneration(ctx context.Context, stop context.Context, run func() error) error {
	wait, cancel := context.WithCancel(ctx)
	defer cancel()
	stopWait := context.AfterFunc(stop, cancel)
	defer stopWait()

	if err := nativeGate.Acquire(wait, 1); err != nil {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := stop.Err(); err != nil {
			return err
		}
		return err
	}
	defer nativeGate.Release(1)

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := stop.Err(); err != nil {
		return err
	}

	return run()
}

// NewModel loads one reusable native model context.
func NewModel(ctx context.Context, cfg Config) (*Model, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	var handle sd.Context
	err := withNative(ctx, func() error {
		params := sd.ContextParamsInit()
		params.ModelPath = cfg.ModelPath
		params.ClipLPath = cfg.ClipLPath
		params.ClipGPath = cfg.ClipGPath
		params.ClipVisionPath = cfg.ClipVisionPath
		params.T5XXLPath = cfg.T5XXLPath
		params.LLMPath = cfg.LLMPath
		params.LLMVisionPath = cfg.LLMVisionPath
		params.DiffusionModelPath = cfg.DiffusionModelPath
		params.HighNoiseDiffusionModelPath = cfg.HighNoiseDiffusionModelPath
		params.EmbeddingsConnectorsPath = cfg.EmbeddingsConnectorsPath
		params.VAEPath = cfg.VAEPath
		params.AudioVAEPath = cfg.AudioVAEPath
		params.TAESDPath = cfg.TAESDPath
		params.ControlNetPath = cfg.ControlNetPath
		params.PhotoMakerPath = cfg.PhotoMakerPath
		params.TensorTypeRules = cfg.TensorTypeRules
		if cfg.CPUThreads > 0 {
			params.NThreads = cfg.CPUThreads
		}

		cfg.CPUThreads = params.NThreads

		var err error
		handle, err = sd.NewContext(params)
		if err != nil {
			return fmt.Errorf("creating context: %w", err)
		}
		if err := ctx.Err(); err != nil {
			sd.FreeContext(handle)
			handle = 0
			return err
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("new-model: %w", err)
	}

	stop, cancel := context.WithCancel(context.Background())
	m := Model{
		config: cfg,
		ctx:    handle,
		stop:   stop,
		cancel: cancel,
	}

	return &m, nil
}

// Generate runs synchronous text-to-image generation. Calls on this Model are
// serialized, while independent Model contexts may generate concurrently.
// Native execution cannot be interrupted after it starts.
func (m *Model) Generate(ctx context.Context, params GenerateParams) (GeneratedImage, error) {
	if err := params.Validate(); err != nil {
		return GeneratedImage{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unloaded {
		return GeneratedImage{}, errors.New("generate: model is unloaded")
	}
	if err := ctx.Err(); err != nil {
		return GeneratedImage{}, err
	}
	if err := m.stop.Err(); err != nil {
		return GeneratedImage{}, err
	}

	p := sd.ImgGenParamsInit()
	p.Prompt = params.Prompt
	p.NegativePrompt = params.NegativePrompt
	p.Width = int32(params.Width)
	p.Height = int32(params.Height)
	p.Steps = int32(params.Steps)
	p.CFGScale = params.CFGScale
	p.Seed = params.Seed
	p.BatchCount = 1
	p.Strength = params.Strength
	if params.InitImage != nil {
		var err error
		p.InitImage, err = imageToRGB(params.InitImage)
		if err != nil {
			return GeneratedImage{}, err
		}
	}

	if err := ctx.Err(); err != nil {
		return GeneratedImage{}, err
	}
	if err := m.stop.Err(); err != nil {
		return GeneratedImage{}, err
	}

	var raw *sd.SDImage
	err := withGeneration(ctx, m.stop, func() error {
		var err error
		raw, err = sd.GenerateImage(m.ctx, p)
		if err != nil {
			return errors.Join(ErrNativeGeneration, err)
		}
		return nil
	})
	if err != nil {
		return GeneratedImage{}, err
	}

	return encodeImage(raw, params.Seed)
}

// Stop prevents generation calls that have not started from entering
// stable-diffusion.cpp. A native call that already started still runs to
// completion.
func (m *Model) Stop() {
	m.cancel()
}

// Unload releases the native context exactly once.
func (m *Model) Unload() error {
	m.Stop()

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unloaded {
		return nil
	}

	if err := withNative(context.Background(), func() error {
		sd.FreeContext(m.ctx)
		return nil
	}); err != nil {
		return fmt.Errorf("unload: freeing context: %w", err)
	}

	m.ctx = 0
	m.unloaded = true

	return nil
}

// Config returns immutable model configuration.
func (m *Model) Config() Config {
	return m.config
}

// Info returns descriptive model information.
func (m *Model) Info() ModelInfo {
	return ModelInfo{
		ModelPath:          m.config.ModelPath,
		DiffusionModelPath: m.config.DiffusionModelPath,
		CPUThreads:         m.config.CPUThreads,
	}
}

func validateInitImage(src image.Image) error {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 {
		return errors.Join(ErrInvalidRequest, errors.New("init image dimensions must be positive"))
	}
	if width > maxImageDimension || height > maxImageDimension || width > maxImagePixels/height {
		return errors.Join(ErrInvalidRequest, fmt.Errorf("init image dimensions must be at most %dx%d and %d pixels", maxImageDimension, maxImageDimension, maxImagePixels))
	}

	return nil
}

func imageToRGB(src image.Image) (*sd.SDImage, error) {
	if err := validateInitImage(src); err != nil {
		return nil, err
	}

	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	raw := sd.SDImage{
		Width:   uint32(width),
		Height:  uint32(height),
		Channel: 3,
		Data:    make([]byte, width*height*3),
	}

	for y := range height {
		for x := range width {
			r, g, b, _ := src.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
			offset := (y*width + x) * 3
			raw.Data[offset] = byte(r >> 8)
			raw.Data[offset+1] = byte(g >> 8)
			raw.Data[offset+2] = byte(b >> 8)
		}
	}

	return &raw, nil
}

func encodeImage(raw *sd.SDImage, seed int64) (GeneratedImage, error) {
	if raw == nil || raw.Channel != 3 || len(raw.Data) != int(raw.Width*raw.Height*3) {
		return GeneratedImage{}, errors.New("encoding PNG: invalid RGB image")
	}

	rgba := image.NewRGBA(image.Rect(0, 0, int(raw.Width), int(raw.Height)))
	for i, j := 0, 0; i < len(raw.Data); i, j = i+3, j+4 {
		copy(rgba.Pix[j:j+3], raw.Data[i:i+3])
		rgba.Pix[j+3] = 255
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, rgba); err != nil {
		return GeneratedImage{}, fmt.Errorf("encoding PNG: %w", err)
	}

	image := GeneratedImage{
		PNG:    buf.Bytes(),
		Width:  int(raw.Width),
		Height: int(raw.Height),
		Seed:   seed,
	}

	return image, nil
}
