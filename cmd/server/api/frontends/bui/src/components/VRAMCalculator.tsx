import { useState } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { VRAMCalculatorResponse } from '../types';

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
  { value: 0, label: 'None (0)' },
  { value: 1, label: 'FMC or SPC (1)' },
  { value: 2, label: 'FMC + SPC (2)' },
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

  const handleCalculate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!modelUrl.trim()) {
      setError('Please enter a model URL');
      return;
    }

    setLoading(true);
    setError(null);
    setResult(null);

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
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to calculate VRAM');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="page">
      <h2>VRAM Calculator</h2>
      <p className="page-description">
        Calculate VRAM requirements for a model from HuggingFace. Only the model header is fetched, not the entire file.
      </p>

      <form onSubmit={handleCalculate} className="form-card">
        <div className="form-group">
          <label htmlFor="modelUrl">Model URL</label>
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
