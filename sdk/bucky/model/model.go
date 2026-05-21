package model

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ardanlabs/bucky/pkg/whisper"
)

// ModelInfo summarizes the static properties of a loaded whisper
// model. It is populated from whisper.Context accessor calls at
// construction time and never mutated thereafter.
type ModelInfo struct {
	ID             string
	Type           string
	IsMultilingual bool
	NVocab         int32
	NTextCtx       int32
	NAudioCtx      int32
	NMels          int32
}

// =============================================================================

// Model owns a single whisper.Context and exposes the low-level
// transcribe / language-detect primitives that the high-level
// sdk/bucky package layers concurrency on top of.
//
// The whisper.Context itself is single-stream so concurrency
// guarantees are the caller's responsibility; Model only serializes
// its own lifecycle (load / Unload).
type Model struct {
	cfg       Config
	handle    whisper.Context
	modelInfo ModelInfo

	shutdown     sync.Mutex
	shutdownFlag bool
}

// NewModel constructs a Model from cfg. ModelPath must be set; all
// other fields fall back to the defaults defined by Config.WithDefaults.
func NewModel(ctx context.Context, cfg Config) (*Model, error) {
	cfg = cfg.WithDefaults()

	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("new-model: model path is required")
	}

	cp := whisper.ContextDefaultParams()
	if cfg.UseGPU {
		cp.UseGPU = 1
	} else {
		cp.UseGPU = 0
	}
	if cfg.FlashAttn {
		cp.FlashAttn = 1
	}
	cp.GPUDevice = cfg.GPUDevice

	handle, err := whisper.InitFromFileWithParams(cfg.ModelPath, cp)
	if err != nil {
		return nil, fmt.Errorf("new-model: init model %q: %w", cfg.ModelPath, err)
	}

	m := Model{
		cfg:    cfg,
		handle: handle,
		modelInfo: ModelInfo{
			ID:             cfg.ModelPath,
			Type:           whisper.ModelTypeReadable(handle),
			IsMultilingual: whisper.IsMultilingual(handle),
			NVocab:         whisper.NVocab(handle),
			NTextCtx:       whisper.NTextCtx(handle),
			NAudioCtx:      whisper.NAudioCtx(handle),
			NMels:          whisper.ModelNMels(handle),
		},
	}

	cfg.Log(ctx, "bucky-new-model",
		"model", cfg.ModelPath,
		"model-type", m.modelInfo.Type,
		"multilingual", m.modelInfo.IsMultilingual,
		"use-gpu", cfg.UseGPU,
		"flash-attn", cfg.FlashAttn,
	)

	return &m, nil
}

// Config returns the resolved Config the Model was built with
// (defaults applied).
func (m *Model) Config() Config {
	return m.cfg
}

// ModelInfo returns the static information about the loaded model.
func (m *Model) ModelInfo() ModelInfo {
	return m.modelInfo
}

// Unload releases the underlying whisper context. Unload is
// single-use per Model; subsequent calls return an error.
//
// The supplied ctx is accepted for parity with sdk/kronk.Model.Unload
// — whisper has no in-flight requests to drain at this layer because
// concurrency is owned by the sdk/bucky wrapper.
func (m *Model) Unload(ctx context.Context) error {
	_ = ctx

	m.shutdown.Lock()
	defer m.shutdown.Unlock()

	if m.shutdownFlag {
		return fmt.Errorf("unload: already unloaded")
	}

	whisper.Free(m.handle)
	m.handle = 0
	m.shutdownFlag = true

	return nil
}

// SystemInfo returns the whisper.cpp system info string parsed into a
// key/value map for observability output. The format mirrors
// sdk/kronk's SystemInfo.
func SystemInfo() map[string]string {
	result := make(map[string]string)

	for part := range strings.SplitSeq(whisper.PrintSystemInfo(), "|") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if idx := strings.Index(part, "="); idx != -1 {
			part = strings.TrimSpace(part[:idx])
		}

		switch kv := strings.SplitN(part, ":", 2); len(kv) {
		case 2:
			key := strings.TrimSpace(kv[0])
			value := strings.TrimSpace(kv[1])
			result[key] = value
		default:
			result[part] = "on"
		}
	}

	return result
}
