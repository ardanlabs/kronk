package kronk

import (
	"strconv"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	defContextWindow = 4 * 1024
)

// ModelConfig represents model level configuration. These values if configured
// incorrectly can cause the system to panic. The defaults are
// used when these values are set to 0.
//
// ContextWindow when set to 0 will use the model's default context window. If
// the model's default context window can't be identified, then a default
// context window of 4k will be used.
//
// Embeddings is a boolean that determines if the model you are using is an
// embedding model. This must be true when using an embedding model.
//
// Device is the device to use for the model. If not set, the default device
// will be used. To see what devices are available, run the following command
// which will be found where you installed llamacpp.
// $ llama-bench --list-devices
type ModelConfig struct {
	ContextWindow int
	Embeddings    bool
	Device        string
}

func adjustConfig(cfg ModelConfig, model llama.Model) ModelConfig {
	cfg = adjustContextWindow(cfg, model)

	return cfg
}

func adjustContextWindow(cfg ModelConfig, model llama.Model) ModelConfig {
	modelCW := defContextWindow
	v, found := searchModelMeta(model, "context_length")
	if found {
		ctxLen, err := strconv.Atoi(v)
		if err == nil {
			modelCW = ctxLen
		}
	}

	if cfg.ContextWindow <= 0 {
		cfg.ContextWindow = modelCW
	}

	return cfg
}

func modelCtxParams(cfg ModelConfig) llama.ContextParams {
	ctxParams := llama.ContextDefaultParams()

	if cfg.Embeddings {
		ctxParams.Embeddings = 1
	}

	// When NBatch is > 64k I get a panic using vision models.
	// So I will limit it to 64k for now.

	if cfg.ContextWindow > 0 {
		ctxParams.NBatch = min(uint32(cfg.ContextWindow), 64*1024)
		ctxParams.NUbatch = uint32(cfg.ContextWindow)
		ctxParams.NCtx = uint32(cfg.ContextWindow)
	}

	return ctxParams
}

func searchModelMeta(model llama.Model, find string) (string, bool) {
	count := llama.ModelMetaCount(model)

	for i := range count {
		key, ok := llama.ModelMetaKeyByIndex(model, i)
		if !ok {
			continue
		}

		if strings.Contains(key, find) {
			value, ok := llama.ModelMetaValStrByIndex(model, i)
			if !ok {
				continue
			}

			return value, true
		}
	}

	return "", false
}
