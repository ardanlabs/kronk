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
	MotionModulePath            string
	ADetailerPath               string
	PhotoMakerPath              string
	TensorTypeRules             string
	Concurrency                 int
	QueueDepth                  int
	AdmissionTimeout            time.Duration
	CPUThreads                  int32
	LinearScale                 float32
	AttnScale                   float32
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

// WithControlNetPath sets a ControlNet model path.
func WithControlNetPath(path string) Option {
	return func(cfg *Config) {
		cfg.ControlNetPath = path
	}
}

// WithMotionModulePath sets an AnimateDiff motion-module path.
func WithMotionModulePath(path string) Option {
	return func(cfg *Config) {
		cfg.MotionModulePath = path
	}
}

// WithADetailerPath sets an ADetailer detector model path.
func WithADetailerPath(path string) Option {
	return func(cfg *Config) {
		cfg.ADetailerPath = path
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

// WithLinearScale sets the linear-operation numerical scale override. Zero
// preserves the model default.
func WithLinearScale(scale float32) Option {
	return func(cfg *Config) {
		cfg.LinearScale = scale
	}
}

// WithAttnScale sets the attention numerical scale override used with flash
// attention. Zero preserves the model default.
func WithAttnScale(scale float32) Option {
	return func(cfg *Config) {
		cfg.AttnScale = scale
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
	if cfg.LinearScale < 0 || cfg.LinearScale != 0 && !finite(float64(cfg.LinearScale)) {
		return errors.New("linear scale must be zero or positive and finite")
	}
	if cfg.AttnScale < 0 || cfg.AttnScale != 0 && !finite(float64(cfg.AttnScale)) {
		return errors.New("attention scale must be zero or positive and finite")
	}

	return nil
}

// GenerateParams controls one text-to-image generation.
type GenerateParams struct {
	Prompt          string
	NegativePrompt  string
	Width           int
	Height          int
	Steps           int
	CFGScale        float32
	Seed            int64
	InitImage       image.Image
	Strength        float32
	ControlImage    image.Image
	ControlStrength float32
	Canny           *CannyParams
}

// CannyParams controls edge detection applied to a ControlNet image.
type CannyParams struct {
	HighThreshold float32
	LowThreshold  float32
	Weak          float32
	Strong        float32
	Inverse       bool
}

// NewCannyParams returns the defaults used by the curated Canny ControlNet.
func NewCannyParams() CannyParams {
	return CannyParams{HighThreshold: 0.08, LowThreshold: 0.08, Weak: 0.8, Strong: 1}
}

// NewGenerateParams returns stable-diffusion.cpp generation defaults.
func NewGenerateParams() GenerateParams {
	return GenerateParams{
		Width:           512,
		Height:          512,
		Steps:           20,
		CFGScale:        7,
		Seed:            -1,
		Strength:        0.75,
		ControlStrength: 1,
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
	if p.ControlImage != nil {
		if err := validateInitImage(p.ControlImage); err != nil {
			return err
		}
		if p.ControlStrength <= 0 || !finite(float64(p.ControlStrength)) {
			return errors.Join(ErrInvalidRequest, errors.New("control strength must be positive and finite"))
		}
		if p.Canny != nil {
			values := []float32{p.Canny.HighThreshold, p.Canny.LowThreshold, p.Canny.Weak, p.Canny.Strong}
			for _, value := range values {
				if value < 0 || value > 1 || !finite(float64(value)) {
					return errors.Join(ErrInvalidRequest, errors.New("canny values must be finite and between 0 and 1"))
				}
			}
		}
	}

	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// DetailParams controls one ADetailer face-refinement pass.
type DetailParams struct {
	Image          image.Image
	Prompt         string
	NegativePrompt string
	ExtraArgs      string
	Steps          int
	CFGScale       float32
	Seed           int64
}

// NewDetailParams returns ADetailer refinement defaults.
func NewDetailParams() DetailParams {
	return DetailParams{ExtraArgs: "input_size=640,confidence=0.3,inpaint_width=64,inpaint_height=64", Steps: 20, CFGScale: 7, Seed: -1}
}

// Validate checks whether parameters describe a supported ADetailer request.
func (p DetailParams) Validate() error {
	if p.Image == nil {
		return errors.Join(ErrInvalidRequest, errors.New("detail image is required"))
	}

	bounds := p.Image.Bounds()
	if bounds.Dx()%8 != 0 || bounds.Dy()%8 != 0 {
		return errors.Join(ErrInvalidRequest, errors.New("detail image dimensions must be multiples of 8"))
	}

	request := NewGenerateParams()
	request.Prompt = p.Prompt
	request.NegativePrompt = p.NegativePrompt
	request.Width = bounds.Dx()
	request.Height = bounds.Dy()
	request.Steps = p.Steps
	request.CFGScale = p.CFGScale
	request.Seed = p.Seed

	return request.Validate()
}

// VideoParams controls one AnimateDiff generation.
type VideoParams struct {
	Prompt         string
	NegativePrompt string
	Width          int
	Height         int
	Steps          int
	Seed           int64
	Frames         int
	FPS            int
}

// NewVideoParams returns conservative AnimateDiff generation defaults.
func NewVideoParams() VideoParams {
	return VideoParams{Width: 128, Height: 128, Steps: 4, Seed: -1, Frames: 4, FPS: 1}
}

// Validate checks whether parameters describe a supported video-generation request.
func (p VideoParams) Validate() error {
	request := NewGenerateParams()
	request.Prompt = p.Prompt
	request.NegativePrompt = p.NegativePrompt
	request.Width = p.Width
	request.Height = p.Height
	request.Steps = p.Steps
	request.Seed = p.Seed

	if err := request.Validate(); err != nil {
		return err
	}

	if p.Frames < 1 || p.Frames > 1_000 {
		return errors.Join(ErrInvalidRequest, errors.New("video frames must be between 1 and 1000"))
	}

	if p.FPS < 1 || p.FPS > 1_000 {
		return errors.Join(ErrInvalidRequest, errors.New("video FPS must be between 1 and 1000"))
	}

	return nil
}

// Audio contains generated interleaved floating-point samples.
type Audio struct {
	SampleRate uint32
	Channels   uint32
	Data       []float32
}

// GeneratedVideo contains owned frames and optional generated audio.
type GeneratedVideo struct {
	Frames []image.Image
	Audio  *Audio
	FPS    int
	Seed   int64
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
	MotionModulePath   string
	ADetailerPath      string
	ModelVersion       string
	CPUThreads         int32
}

// Model owns exactly one native stable-diffusion context.
type Model struct {
	mu       sync.Mutex
	config   Config
	ctx      sd.Context
	detailer sd.ADetailerContext
	version  string
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
	var detailer sd.ADetailerContext
	var modelVersion string

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
		params.MotionModulePath = cfg.MotionModulePath
		params.PhotoMakerPath = cfg.PhotoMakerPath
		params.TensorTypeRules = cfg.TensorTypeRules
		params.LinearScale = cfg.LinearScale
		params.AttnScale = cfg.AttnScale
		if cfg.CPUThreads > 0 {
			params.NThreads = cfg.CPUThreads
		}

		cfg.CPUThreads = params.NThreads

		var err error
		handle, err = sd.NewContext(params)
		if err != nil {
			return fmt.Errorf("creating context: %w", err)
		}

		supported, err := sd.ContextSupportsImageGeneration(handle)
		if err != nil {
			sd.FreeContext(handle)
			handle = 0
			return fmt.Errorf("checking image generation support: %w", err)
		}

		if !supported {
			sd.FreeContext(handle)
			handle = 0
			return errors.New("loaded context does not support image generation")
		}

		modelVersion, err = sd.ModelVersionName(handle)
		if err != nil && !errors.Is(err, sd.ErrUnsupportedAPI) {
			sd.FreeContext(handle)
			handle = 0
			return fmt.Errorf("reading model version: %w", err)
		}

		if cfg.ADetailerPath != "" {
			detailer, err = sd.NewADetailerContext(cfg.ADetailerPath, params.NThreads, "cpu", "")
			if err != nil {
				sd.FreeContext(handle)
				handle = 0
				return fmt.Errorf("creating ADetailer context: %w", err)
			}
		}

		if err := ctx.Err(); err != nil {
			if detailer != 0 {
				sd.FreeADetailerContext(detailer)
				detailer = 0
			}
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
		config:   cfg,
		ctx:      handle,
		detailer: detailer,
		version:  modelVersion,
		stop:     stop,
		cancel:   cancel,
	}

	return &m, nil
}

// Generate runs synchronous text-to-image generation. Calls on this Model are
// serialized, while independent Model contexts may generate concurrently.
// Canceling ctx interrupts native generation and resets the context for reuse.
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

	if params.ControlImage != nil {
		if m.config.ControlNetPath == "" {
			return GeneratedImage{}, errors.Join(ErrInvalidRequest, errors.New("control image requires a ControlNet model"))
		}

		var err error
		p.ControlImage, err = imageToRGB(params.ControlImage)
		if err != nil {
			return GeneratedImage{}, err
		}

		p.ControlStrength = params.ControlStrength
	}

	if err := ctx.Err(); err != nil {
		return GeneratedImage{}, err
	}
	if err := m.stop.Err(); err != nil {
		return GeneratedImage{}, err
	}

	var raw *sd.SDImage
	err := m.runGeneration(ctx, func() error {
		if p.ControlImage != nil && params.Canny != nil {
			if err := sd.PreprocessCanny(p.ControlImage, sd.CannyParams{
				HighThreshold: params.Canny.HighThreshold,
				LowThreshold:  params.Canny.LowThreshold,
				Weak:          params.Canny.Weak,
				Strong:        params.Canny.Strong,
				Inverse:       params.Canny.Inverse,
			}); err != nil {
				return fmt.Errorf("preprocessing Canny image: %w", err)
			}
		}

		var err error
		raw, err = sd.GenerateImage(m.ctx, p)

		return err
	})
	if err != nil {
		return GeneratedImage{}, err
	}

	return encodeImage(raw, params.Seed)
}

func (m *Model) runGeneration(ctx context.Context, generate func() error) error {
	return withGeneration(ctx, m.stop, func() error {
		type cancelResult struct {
			requested bool
			err       error
		}

		nativeDone := make(chan struct{})
		cancelDone := make(chan cancelResult, 1)
		go func() {
			select {
			case <-ctx.Done():
				cancelDone <- cancelResult{requested: true, err: sd.CancelGeneration(m.ctx, sd.CancelAll)}
			case <-m.stop.Done():
				cancelDone <- cancelResult{requested: true, err: sd.CancelGeneration(m.ctx, sd.CancelAll)}
			case <-nativeDone:
				cancelDone <- cancelResult{}
			}
		}()

		generateErr := generate()
		close(nativeDone)

		canceled := <-cancelDone
		var cancelErr error
		if canceled.requested {
			cancelErr = errors.Join(canceled.err, sd.CancelGeneration(m.ctx, sd.CancelReset))
		}

		if err := ctx.Err(); err != nil {
			return errors.Join(err, cancelErr)
		}

		if err := m.stop.Err(); err != nil {
			return errors.Join(err, cancelErr)
		}

		if cancelErr != nil {
			return fmt.Errorf("canceling generation: %w", cancelErr)
		}

		if generateErr != nil {
			return errors.Join(ErrNativeGeneration, generateErr)
		}

		return nil
	})
}

// Detail detects faces and returns the final refined image.
func (m *Model) Detail(ctx context.Context, params DetailParams) (GeneratedImage, error) {
	if err := params.Validate(); err != nil {
		return GeneratedImage{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.unloaded {
		return GeneratedImage{}, errors.New("detail: model is unloaded")
	}

	if m.detailer == 0 {
		return GeneratedImage{}, errors.Join(ErrInvalidRequest, errors.New("detail operation requires an ADetailer model"))
	}

	input, err := imageToRGB(params.Image)
	if err != nil {
		return GeneratedImage{}, err
	}

	inpaint := sd.ImgGenParamsInit()
	inpaint.Prompt = params.Prompt
	inpaint.NegativePrompt = params.NegativePrompt
	inpaint.Width = int32(params.Image.Bounds().Dx())
	inpaint.Height = int32(params.Image.Bounds().Dy())
	inpaint.Steps = int32(params.Steps)
	inpaint.CFGScale = params.CFGScale
	inpaint.Seed = params.Seed

	var images []*sd.SDImage
	err = m.runGeneration(ctx, func() error {
		var err error
		images, err = sd.ADetailImage(m.detailer, m.ctx, input, sd.ADetailerParams{
			Prompt:         params.Prompt,
			NegativePrompt: params.NegativePrompt,
			ExtraArgs:      params.ExtraArgs,
		}, inpaint)
		return err
	})
	if err != nil {
		return GeneratedImage{}, err
	}

	if len(images) == 0 {
		return GeneratedImage{}, errors.Join(ErrNativeGeneration, errors.New("ADetailer returned no images"))
	}

	return encodeImage(images[len(images)-1], params.Seed)
}

// GenerateVideo runs synchronous AnimateDiff generation.
func (m *Model) GenerateVideo(ctx context.Context, params VideoParams) (GeneratedVideo, error) {
	if err := params.Validate(); err != nil {
		return GeneratedVideo{}, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.unloaded {
		return GeneratedVideo{}, errors.New("generate-video: model is unloaded")
	}

	if m.config.MotionModulePath == "" {
		return GeneratedVideo{}, errors.Join(ErrInvalidRequest, errors.New("video generation requires a motion module"))
	}

	p, err := sd.VideoGenParamsInit()
	if err != nil {
		return GeneratedVideo{}, fmt.Errorf("initialize video parameters: %w", err)
	}

	p.Prompt = params.Prompt
	p.NegativePrompt = params.NegativePrompt
	p.Width = int32(params.Width)
	p.Height = int32(params.Height)
	p.Sample.Steps = int32(params.Steps)
	p.Seed = params.Seed
	p.VideoFrames = int32(params.Frames)
	p.FPS = int32(params.FPS)

	var raw []*sd.SDImage
	var audio *sd.Audio
	err = m.runGeneration(ctx, func() error {
		var err error
		raw, audio, err = sd.GenerateVideo(m.ctx, p)
		return err
	})
	if err != nil {
		return GeneratedVideo{}, err
	}

	frames := make([]image.Image, len(raw))
	for i, frame := range raw {
		frames[i], err = decodeImage(frame)
		if err != nil {
			return GeneratedVideo{}, fmt.Errorf("decode frame %d: %w", i, err)
		}
	}

	result := GeneratedVideo{Frames: frames, FPS: params.FPS, Seed: params.Seed}
	if audio != nil {
		result.Audio = &Audio{
			SampleRate: audio.SampleRate,
			Channels:   audio.Channels,
			Data:       append([]float32(nil), audio.Data...),
		}
	}

	return result, nil
}

// Stop prevents generation calls that have not started from entering
// stable-diffusion.cpp and requests cancellation of an active generation.
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
		if m.detailer != 0 {
			sd.FreeADetailerContext(m.detailer)
			m.detailer = 0
		}
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
		MotionModulePath:   m.config.MotionModulePath,
		ADetailerPath:      m.config.ADetailerPath,
		ModelVersion:       m.version,
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
	rgba, err := decodeImage(raw)
	if err != nil {
		return GeneratedImage{}, fmt.Errorf("encoding PNG: %w", err)
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

func decodeImage(raw *sd.SDImage) (*image.RGBA, error) {
	if raw == nil || raw.Channel != 3 || len(raw.Data) != int(raw.Width*raw.Height*3) {
		return nil, errors.New("invalid RGB image")
	}

	rgba := image.NewRGBA(image.Rect(0, 0, int(raw.Width), int(raw.Height)))
	for i, j := 0, 0; i < len(raw.Data); i, j = i+3, j+4 {
		copy(rgba.Pix[j:j+3], raw.Data[i:i+3])
		rgba.Pix[j+3] = 255
	}

	return rgba, nil
}
