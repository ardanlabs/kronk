import { useEffect, useState } from 'react';
import { api } from '../services/api';
import type { BatchEngineDetail, BatchEngineSlotsResponse, BatchSlotDetail } from '../types';
import { labelWithTip } from './ParamTooltips';

function formatSlots(slots: number[] | null | undefined): string {
  return slots && slots.length > 0 ? slots.join(', ') : 'None';
}

function formatSelectedSlot(selected: number): string {
  return selected >= 0 ? `Slot ${selected}` : '—';
}

function nextSlot(model: BatchEngineDetail, eligible: number[] | null | undefined, active: number, cursor: number): string {
  if (model.slots.length === 0) return '—';
  for (let offset = 0; offset < model.slots.length; offset += 1) {
    const candidate = (cursor + offset) % model.slots.length;
    if (candidate !== active && eligible?.includes(candidate)) return `Slot ${candidate}`;
  }
  const fallback = active >= 0 ? (active + 1) % model.slots.length : cursor;
  return `Slot ${fallback}`;
}

function formatAge(ms: number): string {
  if (ms <= 0) return '—';
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`;
  const minutes = Math.floor(ms / 60_000);
  return `${minutes}m ${Math.floor((ms % 60_000) / 1000)}s`;
}

function stage1Progress(slot: BatchSlotDetail): string {
  return slot.phase === 'imc-restore' ? 'Restoring session state' : '—';
}

function stage2Progress(slot: BatchSlotDetail): string {
  if (slot.phase === 'prefill-imc') {
    return `${slot.imc_prepared_tokens.toLocaleString()} / ${slot.imc_total_tokens.toLocaleString()} · IMC extension`;
  }
  if (slot.phase !== 'prefill') return '—';
  const complete = slot.prefilled_tokens;
  const total = complete + slot.prefill_remaining;
  return `${complete.toLocaleString()} / ${total.toLocaleString()}${slot.prefill_owner ? ' (owner)' : ''}`;
}

function requestLabel(slot: BatchSlotDetail): string {
  return slot.request_id.replace(/^chatcmpl-/, '') || '—';
}

function generationLabel(slot: BatchSlotDetail): string {
  if (!slot.generation_mode) return '—';
  return `${slot.generated_tokens.toLocaleString()} tokens · ${slot.generation_mode} · ${slot.generation_rows} rows`;
}

function phaseLabel(phase: BatchSlotDetail['phase']): string {
  if (phase === 'imc-restore') return 'prefill stage 1 · restore';
  if (phase === 'prefill-imc') return 'prefill stage 2 · IMC';
  if (phase === 'prefill') return 'prefill stage 2 · request';
  if (phase === 'media-prefill') return 'prefill stage 2 · media';
  return phase;
}

function modelMode(model: BatchEngineDetail): string {
  if (model.mtp) return `MTP · nDraft ${model.ndraft}`;
  if (model.ndraft > 0) return `Speculative · nDraft ${model.ndraft}`;
  return 'Non-MTP';
}

function ModelSlots({ model }: { model: BatchEngineDetail }) {
  const stage2IMCNext = nextSlot(
    model,
    model.eligible_imc_slots,
    model.imc_selector_selected,
    model.imc_selector_next,
  );
  const stage2Next = nextSlot(
    model,
    model.eligible_prefill_slots,
    model.prefill_selector_selected,
    model.prefill_selector_selected >= 0
      ? (model.prefill_selector_selected + 1) % model.slots.length
      : model.prefill_selector_next,
  );
  return (
    <div className="card slot-model-card">
      <div className="slot-model-heading">
        <div>
          <h3>{model.model_id}</h3>
          <span className="slot-mode-badge">{modelMode(model)}</span>
        </div>
        <span className="slot-iteration">Iteration {model.iteration.toLocaleString()}</span>
      </div>

      <div className="slot-summary-grid">
        <div><span>{labelWithTip('Batch sizing', 'slotBatchSizing')}</span><strong>{model.prefill_batch_size.toLocaleString()} / {model.nubatch.toLocaleString()} / {model.nbatch.toLocaleString()}</strong><small>Prefill batch / NUBatch / NBatch</small></div>
        <div><span>{labelWithTip('Request queue', 'slotQueue')}</span><strong>{model.queued_requests + model.pending_requests}</strong><small>{model.queued_requests} channel + {model.pending_requests} pending</small></div>
        <div><span>{labelWithTip('Prefill stage 2 · IMC extension', 'slotIMCSelector')}</span><strong>{formatSelectedSlot(model.imc_selector_selected)}</strong><small>Next: {stage2IMCNext} · Waiting: {formatSlots(model.eligible_imc_slots)}</small></div>
        <div><span>{labelWithTip('Prefill stage 2 · request', 'slotPrefillSelector')}</span><strong>{formatSelectedSlot(model.prefill_selector_selected)}</strong><small>Next: {stage2Next} · Waiting: {formatSlots(model.eligible_prefill_slots)}</small></div>
      </div>

      <div className="table-container">
        <table className="slots-table">
          <thead>
            <tr>
              <th>{labelWithTip('Slot', 'slotID')}</th>
              <th>{labelWithTip('Phase', 'slotPhase')}</th>
              <th>{labelWithTip('Request', 'slotRequest')}</th>
              <th>{labelWithTip('Age', 'slotAge')}</th>
              <th>{labelWithTip('Stage 1 · restore', 'slotIMCProgress')}</th>
              <th>{labelWithTip('Stage 2 · decode', 'slotPrefillProgress')}</th>
              <th>{labelWithTip('Generation', 'slotGeneration')}</th>
              <th style={{ textAlign: 'right' }}>{labelWithTip('Past', 'slotPastTokens')}</th>
            </tr>
          </thead>
          <tbody>
            {model.slots.map((slot) => (
              <tr key={slot.id} className={slot.prefill_owner ? 'slot-prefill-owner' : undefined}>
                <td><strong>{slot.id}</strong></td>
                <td><span className={`slot-phase slot-phase-${slot.phase}`}>{phaseLabel(slot.phase)}</span></td>
                <td><code className="slot-request-id" title={slot.request_id || undefined}>{requestLabel(slot)}</code></td>
                <td className="slot-request-age">{formatAge(slot.request_age_ms)}</td>
                <td>{stage1Progress(slot)}</td>
                <td>{stage2Progress(slot)}</td>
                <td>{generationLabel(slot)}</td>
                <td style={{ textAlign: 'right' }}>{slot.past_tokens.toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}

export default function Slots() {
  const [data, setData] = useState<BatchEngineSlotsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const loadSlots = async (silent = false) => {
    if (!silent) setLoading(true);
    setError(null);
    try {
      setData(await api.listBatchEngineSlots());
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load batch engine slots');
    } finally {
      if (!silent) setLoading(false);
    }
  };

  useEffect(() => {
    loadSlots();
    const id = window.setInterval(() => loadSlots(true), 1000);
    return () => window.clearInterval(id);
  }, []);

  return (
    <div>
      <div className="page-header-with-action">
        <div>
          <h2>Batch Engine Slots</h2>
          <p className="page-description">Live scheduler, prefill, generation, and IMC preparation state for loaded generation models</p>
        </div>
        <button className="btn btn-primary" onClick={() => loadSlots()} disabled={loading}>Refresh</button>
      </div>

      {error && <div className="alert alert-error">{error}</div>}
      {loading && !data && <div className="loading">Loading slot state…</div>}
      {!loading && data?.length === 0 && (
        <div className="empty-state">No loaded generation models expose batch-engine slots.</div>
      )}
      {data?.map((model) => <ModelSlots key={model.model_id} model={model} />)}
    </div>
  );
}
