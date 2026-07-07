// Adapter lifecycle helpers for model-scoped LoRA support.
//
// This file separates adapter management into two phases:
//
//  1. Model load time (initAdapter): load adapter GGUFs once against the
//     base llama.Model and keep the returned handles on Model.
//  2. Context creation time (setAdaptersOnContext): apply the loaded adapter
//     handles and configured scales to each llama.Context that will decode.
//
// Adapters are therefore initialized once per loaded model and re-applied to
// every runtime context (generation, pooled, draft) derived from that model.
// freeAdapters releases model-owned adapter handles during unload/error paths.
package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// initAdapter loads and validates all configured adapters against the model
// and stores their handles plus effective scales on m.
func initAdapter(ctx context.Context, m *Model) error {
	m.adapters = make([]loadedAdapter, 0, len(m.cfg.Adapters))

	for i, a := range m.cfg.Adapters {
		handle, err := llama.AdapterLoraInit(m.model, a.Path)
		if err != nil {
			m.freeAdapters()
			return fmt.Errorf("adapter[%d] %q: %w", i, a.Path, err)
		}

		// AdapterLoraInit can also return a zero handle with a nil error on a
		// bad, mismatched, or corrupt GGUF, so the handle is checked separately.
		if handle == 0 {
			m.freeAdapters()
			return fmt.Errorf("adapter[%d] %q: load returned a nil handle (mismatched or corrupt GGUF?)", i, a.Path)
		}

		name := a.ModelID
		scale := a.GetScale()

		m.adapters = append(m.adapters, loadedAdapter{
			handle: handle,
			name:   name,
			path:   a.Path,
			scale:  scale,
		})
	}

	m.log(ctx, "init-adapters", "status", "loaded", "count", len(m.adapters))

	return nil
}

// adapterHandles returns adapter handles in stable config order for passing
// to llama.SetAdaptersLora.
func (m *Model) adapterHandles() []llama.AdapterLora {
	handles := make([]llama.AdapterLora, len(m.adapters))
	for i := range m.adapters {
		handles[i] = m.adapters[i].handle
	}
	return handles
}

// adapterScales returns adapter scales aligned with adapterHandles.
func (m *Model) adapterScales() []float32 {
	scales := make([]float32, len(m.adapters))
	for i := range m.adapters {
		scales[i] = m.adapters[i].scale
	}
	return scales
}

// setAdaptersOnContext applies currently loaded adapters to a single llama
// context. It is safe to call on every newly created context.
func (m *Model) setAdaptersOnContext(ctx context.Context, lctx llama.Context) error {
	if len(m.adapters) == 0 {
		return nil
	}

	handles := m.adapterHandles()
	scales := m.adapterScales()
	if rc := llama.SetAdaptersLora(lctx, handles, scales); rc != 0 {
		return fmt.Errorf("set-adapters-lora failed: rc=%d", rc)
	}

	m.log(ctx, "set-adapters", "status", "applied", "count", len(m.adapters))

	return nil
}

// freeAdapters releases all loaded adapter handles and clears in-memory
// adapter state on the model.
func (m *Model) freeAdapters() {
	for i := range m.adapters {
		// AdapterLoraFree only reports an error for zero handles, and a loaded
		// adapter never has one, so there is nothing actionable to propagate.
		llama.AdapterLoraFree(m.adapters[i].handle)
	}

	m.adapters = nil
}
