package bucky

// Option represents a functional option for configuring a Config.
type Option func(*Config)

// NewConfig builds a Config from the supplied options. Zero-valued
// fields are left as zero — defaulting happens inside New /
// NewWithContext.
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

// WithLogger sets the Logger the handle and its operations use.
func WithLogger(v Logger) Option { return func(c *Config) { c.Log = v } }
