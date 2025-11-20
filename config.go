package kronk

import (
	"github.com/hybridgroup/yzma/pkg/llama"
)

type LogType int

const (
	LogSilent LogType = iota + 1
	LogNormal
)

// Config represents model level configuration.
type Config struct {
	LogSet        LogType
	ContextWindow uint32
	Embeddings    bool
}

func (cfg Config) setLog() {
	switch cfg.LogSet {
	case LogSilent:
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
