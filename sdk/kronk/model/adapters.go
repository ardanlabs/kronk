package model

import (
	"context"
	"fmt"

	"github.com/hybridgroup/yzma/pkg/llama"
)

const (
	metaKeyName = "general.name"
	metaKeyArch = "general.architecture"
)

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
		if name == "" {
			if v, ok := llama.AdapterMetaValStr(handle, metaKeyName); ok {
				name = v
			} else {
				name = a.Path
			}
		}

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

func (m *Model) adapterHandles() []llama.AdapterLora {
	handles := make([]llama.AdapterLora, len(m.adapters))
	for i := range m.adapters {
		handles[i] = m.adapters[i].handle
	}
	return handles
}

func (m *Model) adapterScales() []float32 {
	scales := make([]float32, len(m.adapters))
	for i := range m.adapters {
		scales[i] = m.adapters[i].scale
	}
	return scales
}

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

func (m *Model) freeAdapters() {
	for i := range m.adapters {
		// AdapterLoraFree only reports an error for zero handles, and a loaded
		// adapter never has one, so there is nothing actionable to propagate.
		llama.AdapterLoraFree(m.adapters[i].handle)
	}

	m.adapters = nil
}
