package llamacpp

import (
	"github.com/hybridgroup/yzma/pkg/llama"
)

// Config represents model level configuration.
type Config struct {
	LogSet        uintptr
	ContextWindow uint32
	Embeddings    bool
}

func (cfg Config) setLog() {
	switch cfg.LogSet {
	case llama.LogSilent():
		llama.LogSet(llama.LogSilent())
	default:
		llama.LogSet(llama.LogNormal)
	}
}

func (cfg Config) ctxParams() llama.ContextParams {
	ctxParams := llama.ContextDefaultParams()

	if cfg.Embeddings {
		ctxParams.Embeddings = 1
	}

	if cfg.ContextWindow > 0 {
		ctxParams.NBatch = cfg.ContextWindow
		ctxParams.NUbatch = cfg.ContextWindow
		ctxParams.NCtx = cfg.ContextWindow
	}

	return ctxParams
}
