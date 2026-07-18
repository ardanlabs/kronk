package model

import (
	"context"
	"errors"
	"fmt"
	"path"

	"github.com/hybridgroup/yzma/pkg/llama"
)

// loadAdapters loads the configured LoRA adapter handles against the target
// model. No context owns the handles yet, so partial failures can release the
// handles immediately.
func (m *Model) loadAdapters(ctx context.Context) error {
	if len(m.cfg.Adapters) == 0 {
		return nil
	}

	m.adapterHandles = make([]llama.AdapterLora, 0, len(m.cfg.Adapters))
	m.adapterScales = make([]float32, 0, len(m.cfg.Adapters))

	for i, adapter := range m.cfg.Adapters {
		handle, err := llama.AdapterLoraInit(m.model, adapter.Path)
		if err != nil {
			return errors.Join(
				fmt.Errorf("load-adapters: adapter[%d] %q: %w", i, adapter.Path, err),
				m.freeAdapters(),
			)
		}
		if handle == 0 {
			return errors.Join(
				fmt.Errorf("load-adapters: adapter[%d] %q returned an invalid handle", i, adapter.Path),
				m.freeAdapters(),
			)
		}

		m.adapterHandles = append(m.adapterHandles, handle)
		m.adapterScales = append(m.adapterScales, adapter.Scale)
		m.log(ctx, "load-adapter", "status", "loaded", "adapter", path.Base(adapter.Path), "scale", adapter.Scale)
	}

	return nil
}

// applyAdapters registers the loaded adapter handles and their fixed scales on
// a target-model context. It is a no-op when no adapters are configured.
func (m *Model) applyAdapters(lctx llama.Context) error {
	if len(m.adapterHandles) == 0 {
		return nil
	}

	if rc := llama.SetAdaptersLora(lctx, m.adapterHandles, m.adapterScales); rc != 0 {
		return fmt.Errorf("apply-adapters: set adapters failed: rc=%d", rc)
	}

	return nil
}

// applyAdaptersToPool applies the target model's adapters to every context in
// the embedding/reranking pool. The caller owns pool cleanup on failure.
func (m *Model) applyAdaptersToPool() error {
	if m.pool == nil {
		return nil
	}

	for i, lctx := range m.pool.contexts {
		if err := m.applyAdapters(lctx); err != nil {
			return fmt.Errorf("apply-adapters-to-pool: context[%d]: %w", i, err)
		}
	}

	return nil
}

// adapterDraftContext returns the embedded MTP context backed by the target
// model. Drafts backed by a separate model intentionally return zero.
func adapterDraftContext(draft drafter) llama.Context {
	mtp, ok := draft.(*mtpDrafter)
	if !ok {
		return 0
	}

	return mtp.c.lctx
}

// applyAdaptersToDraft applies target adapters only to an embedded MTP
// context, which was created from the target llama model. Classic drafts and
// separate-file MTP assistants own different models and are excluded.
func (m *Model) applyAdaptersToDraft(draft drafter) error {
	lctx := adapterDraftContext(draft)
	if lctx == 0 {
		return nil
	}

	if err := m.applyAdapters(lctx); err != nil {
		return fmt.Errorf("apply-adapters-to-draft: %w", err)
	}

	return nil
}

// freeAdapters releases every loaded adapter handle. Callers must first free
// every context on which the handles were registered and must call this before
// freeing the target model.
func (m *Model) freeAdapters() error {
	errs := make([]error, 0, len(m.adapterHandles))
	for i, handle := range m.adapterHandles {
		if err := llama.AdapterLoraFree(handle); err != nil {
			errs = append(errs, fmt.Errorf("free-adapters: adapter[%d]: %w", i, err))
		}
	}

	m.adapterHandles = nil
	m.adapterScales = nil

	return errors.Join(errs...)
}
