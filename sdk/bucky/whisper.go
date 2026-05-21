package bucky

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ardanlabs/bucky/pkg/whisper"
)

// Version contains the current version of the bucky SDK package.
const Version = "0.1.0"

// =============================================================================

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
	Size           int64
}

// =============================================================================

// Whisper provides a concurrently safe API for using whisper.cpp.
// Each Whisper owns one whisper.Context. The context itself is
// single-stream, so concurrency is bounded by a semaphore sized at
// construction time from Config.NSeqMax * Config.QueueDepth.
type Whisper struct {
	cfg           Config
	handle        whisper.Context
	sem           chan struct{}
	activeStreams atomic.Int32
	shutdown      sync.Mutex
	shutdownFlag  bool
	modelInfo     ModelInfo
}

// New provides the ability to use a whisper model in a concurrently
// safe way.
func New(opts ...Option) (*Whisper, error) {
	return NewWithContext(context.Background(), opts...)
}

// NewWithContext provides the ability to use a whisper model in a
// concurrently safe way. The context is used to support logging trace
// ids during model loading.
func NewWithContext(ctx context.Context, opts ...Option) (*Whisper, error) {
	if libraryLocation == "" {
		return nil, fmt.Errorf("new: the Init() function has not been called")
	}

	cfg := NewConfig(opts...).withDefaults()

	if cfg.ModelPath == "" {
		return nil, fmt.Errorf("new: model path is required")
	}

	// -------------------------------------------------------------------------

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
		return nil, fmt.Errorf("new: init model %q: %w", cfg.ModelPath, err)
	}

	// -------------------------------------------------------------------------

	semCapacity := max(cfg.NSeqMax, 1) * cfg.QueueDepth

	w := Whisper{
		cfg:    cfg,
		handle: handle,
		sem:    make(chan struct{}, semCapacity),
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

	cfg.Log(ctx, "bucky-new",
		"model", cfg.ModelPath,
		"model-type", w.modelInfo.Type,
		"multilingual", w.modelInfo.IsMultilingual,
		"use-gpu", cfg.UseGPU,
		"flash-attn", cfg.FlashAttn,
		"sem-capacity", semCapacity,
	)

	return &w, nil
}

// Config returns a copy of the configuration the handle was built
// with.
func (w *Whisper) Config() Config {
	return w.cfg
}

// ModelInfo returns the static information about the loaded model.
func (w *Whisper) ModelInfo() ModelInfo {
	return w.modelInfo
}

// ActiveStreams returns the number of in-flight transcribe calls.
func (w *Whisper) ActiveStreams() int {
	return int(w.activeStreams.Load())
}

// SystemInfo returns the whisper.cpp system info string parsed into a
// key/value map for observability output. The format mirrors
// sdk/kronk's SystemInfo.
func (w *Whisper) SystemInfo() map[string]string {
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

// Unload releases the underlying whisper context. It blocks until
// outstanding transcribes drain or the supplied context expires.
// Subsequent calls fail; Unload is single-use per handle.
func (w *Whisper) Unload(ctx context.Context) error {
	if _, exists := ctx.Deadline(); !exists {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
	}

	// -------------------------------------------------------------------------

	err := func() error {
		w.shutdown.Lock()
		defer w.shutdown.Unlock()

		if w.shutdownFlag {
			return fmt.Errorf("unload: already unloaded")
		}

		for w.activeStreams.Load() > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("unload: cannot unload, too many active-streams[%d]: %w", w.activeStreams.Load(), ctx.Err())

			case <-time.After(100 * time.Millisecond):
			}
		}

		w.shutdownFlag = true
		return nil
	}()

	if err != nil {
		return err
	}

	// -------------------------------------------------------------------------

	whisper.Free(w.handle)
	w.handle = 0

	return nil
}
