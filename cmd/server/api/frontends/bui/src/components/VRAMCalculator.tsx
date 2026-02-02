import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { VRAMCalculatorResponse } from '../types';

const VRAM_FORMULA_CONTENT = `SLOT MEMORY AND TOTAL VRAM COST FORMULA

These figures are for KV cache VRAM only (when offload-kqv: true).
Model weights require additional VRAM: ~7GB (7B Q8) or ~70GB (70B Q8).
Total VRAM = model weights + KV cache.

Memory is statically allocated upfront when the model loads,
based on n_ctx × n_seq_max. Reserving slots consumes memory whether or not
they're actually used.

Example Calculations:

This is how you calculate the amount of KV memory you need per slot.

KV_Per_Token_Per_Layer = head_count_kv × (key_length + value_length) × bytes_per_element
KV_Per_Slot            = n_ctx × n_layers × KV_per_token_per_layer

------------------------------------------------------------------------------
So Given these values, this is what you are looking at:

Model   Context_Window   KV_Per_Slot      NSeqMax (Slots)
7B      8K               ~537 MB VRAM     2
70B     8K               ~1.3 GB VRAM     2

No Caching:
Total sequences allocated: 2 (no cache)
7B:  Slot Memory (2 × 537MB) ~1.07GB: Total VRAM: ~8.1GB
70B: Slot Memory (2 × 1.3GB) ~2.6GB : Total VRAM: ~72.6GB

Incremental Memory Caching (IMC) or System Prompt Cache (SPC):
Total sequences allocated: 2 + 1 = 3 (cache)
7B:  Slot Memory (3 × 537MB) ~1.6GB: Total VRAM: ~8.6GB
70B: Slot Memory (3 × 1.3GB) ~3.9GB: Total VRAM: ~73.9GB

Note: SPC and FMC are mutually exclusive; FMC caches system + user together.

------------------------------------------------------------------------------
Full Example With Real Model:

Model                   : Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
Size                    : 36.0GB
Context Window          : 131072 (128k)
cache-type-k            : q8_0 (1 byte per element), f16 (2 bytes)
cache-type-v            : q8_0 (1 byte per element), f16 (2 bytes)
block_count             : 48  (n_layers)
attention.head_count_kv : 4   (KV heads)
attention.key_length    : 128 (K dimension per head)
attention.value_length  : 128 (V dimension per head)

KV_per_token_per_layer = head_count_kv  ×  (key_length + value_length)  ×  bytes_per_element
1024 bytes             =             4  ×  ( 128       +         128 )  ×  1

KV_Per_Slot            =  n_ctx  ×  n_layers  ×  KV_per_token_per_layer
~6.4 GB                =  131072 ×  48        ×  1024

No Caching:
Total sequences allocated: 2 : (no cache)
Slot Memory (2 × 6.4GB) ~12.8GB: Total VRAM: ~48.8GB

Incremental Memory Caching (IMC) or System Prompt Cache (SPC):
Total sequences allocated: 3 : (2 + 1) (1 cache sequence)
Slot Memory (3 × 6.4GB) ~19.2GB: Total VRAM: ~55.2GB

Note: SPC and IMC are mutually exclusive; FMC caches system + user together.`;

const CONTEXT_WINDOW_OPTIONS = [
  { value: 1024, label: '1K' },
  { value: 2048, label: '2K' },
  { value: 4096, label: '4K' },
  { value: 8192, label: '8K' },
  { value: 16384, label: '16K' },
  { value: 32768, label: '32K' },
  { value: 65536, label: '64K' },
  { value: 131072, label: '128K' },
  { value: 262144, label: '256K' },
];

const BYTES_PER_ELEMENT_OPTIONS = [
  { value: 4, label: 'f32 (4 bytes)' },
  { value: 2, label: 'f16 / bf16 (2 bytes)' },
  { value: 1, label: 'q8_0 / q4_0 / q4_1 / q5_0 / q5_1 (1 byte)' },
];

const SLOT_OPTIONS = [1, 2, 3, 4, 5];

const CACHE_SEQUENCE_OPTIONS = [
  { value: 0, label: 'Off' },
  { value: 1, label: 'On (IMC or SPC)' },
];

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${(bytes / Math.pow(k, i)).toFixed(2)} ${sizes[i]}`;
}

export default function VRAMCalculator() {
  const { token } = useToken();
  const [modelUrl, setModelUrl] = useState('');
  const [contextWindow, setContextWindow] = useState(8192);
  const [bytesPerElement, setBytesPerElement] = useState(1);
  const [slots, setSlots] = useState(2);
  const [cacheSequences, setCacheSequences] = useState(0);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<VRAMCalculatorResponse | null>(null);
  const [showLearnMore, setShowLearnMore] = useState(false);
  const hasCalculated = useRef(false);

  const performCalculation = useCallback(async (clearResult = true) => {
    if (!modelUrl.trim()) {
      setError('Please enter a model URL');
      return;
    }

    setLoading(true);
    setError(null);
    if (clearResult) {
      setResult(null);
    }

    try {
      const response = await api.calculateVRAM(
        {
          model_url: modelUrl.trim(),
          context_window: contextWindow,
          bytes_per_element: bytesPerElement,
          slots: slots,
          cache_sequences: cacheSequences,
        },
        token || undefined
      );
      setResult(response);
      hasCalculated.current = true;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to calculate VRAM');
    } finally {
      setLoading(false);
    }
  }, [modelUrl, contextWindow, bytesPerElement, slots, cacheSequences, token]);

  useEffect(() => {
    if (hasCalculated.current && modelUrl.trim()) {
      performCalculation(false);
    }
  }, [contextWindow, bytesPerElement, slots, cacheSequences]);

  const handleCalculate = async (e: React.FormEvent) => {
    e.preventDefault();
    await performCalculation();
  };

  return (
    <div className="page">
      <div className="page-header-with-action">
        <div>
          <h2>VRAM Calculator</h2>
          <p className="page-description">
            Calculate VRAM requirements for a model from HuggingFace. Only the model header is fetched, not the entire file.
          </p>
        </div>
        <button
          type="button"
          className="btn btn-secondary"
          onClick={() => setShowLearnMore(true)}
        >
          Learn More
        </button>
      </div>

      {showLearnMore && (
        <div className="modal-overlay" onClick={() => setShowLearnMore(false)}>
          <div className="modal-content modal-large" onClick={(e) => e.stopPropagation()}>
            <div className="modal-header">
              <h3>VRAM Calculation Formula</h3>
              <button
                className="modal-close"
                onClick={() => setShowLearnMore(false)}
                aria-label="Close"
              >
                ×
              </button>
            </div>
            <div className="modal-body">
              <pre className="vram-formula-content">{VRAM_FORMULA_CONTENT}</pre>
            </div>
          </div>
        </div>
      )}

      <form onSubmit={handleCalculate} className="form-card">
        <div className="form-group">
          <label htmlFor="modelUrl">Model URL (download link or org/family/file)</label>
          <input
            id="modelUrl"
            type="text"
            value={modelUrl}
            onChange={(e) => setModelUrl(e.target.value)}
            placeholder="https://huggingface.co/org/model/resolve/main/model.gguf"
            className="form-input"
          />
          <small className="form-hint">
            Enter a HuggingFace URL to a GGUF model file
          </small>
        </div>

        <div className="form-group">
          <label htmlFor="contextWindow">Context Window</label>
          <select
            id="contextWindow"
            value={contextWindow}
            onChange={(e) => setContextWindow(Number(e.target.value))}
            className="form-select"
          >
            {CONTEXT_WINDOW_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label} ({opt.value.toLocaleString()} tokens)
              </option>
            ))}
          </select>
        </div>

        <div className="form-group">
          <label htmlFor="bytesPerElement">Cache Type (Bytes Per Element)</label>
          <select
            id="bytesPerElement"
            value={bytesPerElement}
            onChange={(e) => setBytesPerElement(Number(e.target.value))}
            className="form-select"
          >
            {BYTES_PER_ELEMENT_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
        </div>

        <div className="form-group">
          <label htmlFor="slots">Slots (Concurrent Sequences)</label>
          <select
            id="slots"
            value={slots}
            onChange={(e) => setSlots(Number(e.target.value))}
            className="form-select"
          >
            {SLOT_OPTIONS.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </div>

        <div className="form-group">
          <label htmlFor="cacheSequences">Cache Sequences</label>
          <select
            id="cacheSequences"
            value={cacheSequences}
            onChange={(e) => setCacheSequences(Number(e.target.value))}
            className="form-select"
          >
            {CACHE_SEQUENCE_OPTIONS.map((opt) => (
              <option key={opt.value} value={opt.value}>
                {opt.label}
              </option>
            ))}
          </select>
          <small className="form-hint">
            FMC = First Message Cache, SPC = System Prompt Cache
          </small>
        </div>

        <button type="submit" className="btn btn-primary" disabled={loading}>
          {loading ? 'Calculating...' : 'Calculate VRAM'}
        </button>
      </form>

      {error && <div className="alert alert-error">{error}</div>}

      {result && (
        <div className="card vram-results">
          <h3>VRAM Calculation Results</h3>
          <div className="vram-results-grid">
            <div className="vram-result-item">
              <span className="vram-result-label">Total VRAM Required</span>
              <span className="vram-result-value vram-result-total">
                {formatBytes(result.total_vram)}
              </span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">Slot Memory (KV Cache)</span>
              <span className="vram-result-value">{formatBytes(result.slot_memory)}</span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">KV Per Slot</span>
              <span className="vram-result-value">{formatBytes(result.kv_per_slot)}</span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">Total Slots</span>
              <span className="vram-result-value">{result.total_slots}</span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">KV Per Token Per Layer</span>
              <span className="vram-result-value">{formatBytes(result.kv_per_token_per_layer)}</span>
            </div>
          </div>

          <h4 style={{ marginTop: '2rem' }}>Model Metadata (from GGUF header)</h4>
          <div className="vram-results-grid">
            <div className="vram-result-item">
              <span className="vram-result-label">Model Size</span>
              <span className="vram-result-value">{formatBytes(result.input.model_size_bytes)}</span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">Block Count (Layers)</span>
              <span className="vram-result-value">{result.input.block_count}</span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">Head Count KV</span>
              <span className="vram-result-value">{result.input.head_count_kv}</span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">Key Length</span>
              <span className="vram-result-value">{result.input.key_length}</span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">Value Length</span>
              <span className="vram-result-value">{result.input.value_length}</span>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
