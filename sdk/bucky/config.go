package bucky

// Config carries the per-handle whisper.cpp configuration. Fields are
// resolved through the functional Option pattern (see options.go) at
// construction time and treated as read-only thereafter.
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
	// backend). Defaults to true so behavior matches whisper.cpp's own
	// defaults.
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

	// Log is the logger the handle uses for diagnostic output.
	// Defaults to DiscardLogger when nil.
	Log Logger
}

// withDefaults returns cfg with the zero-valued fields filled in.
func (cfg Config) withDefaults() Config {
	if cfg.NSeqMax <= 0 {
		cfg.NSeqMax = 1
	}
	if cfg.QueueDepth <= 0 {
		cfg.QueueDepth = 2
	}
	if cfg.Log == nil {
		cfg.Log = DiscardLogger
	}
	return cfg
}
