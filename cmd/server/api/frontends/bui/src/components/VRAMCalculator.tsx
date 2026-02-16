import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { VRAMCalculatorResponse } from '../types';

const VRAM_FORMULA_CONTENT = `VRAM CALCULATION FORMULA

Total VRAM = Model Weights + KV Cache

Model weights are determined by the GGUF file size (e.g., ~8GB for a
7B Q8_0 model). The KV cache is the variable cost you control through
configuration.

==============================================================================
SLOTS AND SEQUENCES
==============================================================================

A slot is a processing unit that handles one request at a time. Each slot
is assigned a unique sequence ID that maps to an isolated partition in the
shared KV cache. The mapping is always 1:1:

  NSeqMax = 4 (set via n_seq_max in model config)

  Slot 0  →  Sequence 0  →  KV cache partition 0
  Slot 1  →  Sequence 1  →  KV cache partition 1
  Slot 2  →  Sequence 2  →  KV cache partition 2
  Slot 3  →  Sequence 3  →  KV cache partition 3

NSeqMax controls how many slots (and sequences) are created. More slots
means more concurrent requests, but each slot reserves its own KV cache
partition in VRAM whether or not it is actively used.

==============================================================================
WHAT AFFECTS KV CACHE MEMORY PER SEQUENCE
==============================================================================

Each sequence's KV cache partition size is determined by three factors:

  1. Context Window (n_ctx)
     The maximum number of tokens the sequence can hold. Larger context
     windows linearly increase memory. 32K context uses 4× the memory
     of 8K context.

  2. Number of Layers (block_count)
     Every transformer layer stores its own key and value tensors per
     token. More layers means more memory per token. A 70B model with
     80 layers uses ~2.5× more per-token memory than a 7B model with
     32 layers.

  3. KV Cache Precision (bytes_per_element)
     The data type used to store cached keys and values:
       f16  = 2 bytes per element (default, best quality)
       q8_0 = 1 byte per element  (50% VRAM savings, good quality)
     The head geometry (head_count_kv, key_length, value_length) is
     fixed by the model architecture and read from the GGUF header.

The formula:

  KV_Per_Token_Per_Layer = head_count_kv × (key_length + value_length) × bytes_per_element
  KV_Per_Sequence        = n_ctx × n_layers × KV_Per_Token_Per_Layer

==============================================================================
WHAT AFFECTS TOTAL KV CACHE (SLOT MEMORY)
==============================================================================

Total KV cache (Slot Memory) is simply the per-sequence cost multiplied
by the number of slots:

  Slot_Memory = NSeqMax × KV_Per_Sequence
  Total_VRAM  = Model_Weights + Slot_Memory

Memory is statically allocated upfront when the model loads. All slots
reserve their full KV cache partition regardless of whether they are
actively processing a request.

==============================================================================
EXAMPLE: REAL MODEL CALCULATION
==============================================================================

Model                   : Qwen3-Coder-30B-A3B-Instruct-UD-Q8_K_XL
Model Weights           : 36.0 GB
Context Window (n_ctx)  : 131,072 (128K)
Bytes Per Element       : 1 (q8_0)
block_count (n_layers)  : 48
attention.head_count_kv : 4
attention.key_length    : 128
attention.value_length  : 128

Step 1 — Per-token-per-layer cost:

  KV_Per_Token_Per_Layer = 4 × (128 + 128) × 1 = 1,024 bytes

Step 2 — Per-sequence cost:

  KV_Per_Sequence = 131,072 × 48 × 1,024 = ~6.4 GB

Step 3 — Total KV cache (NSeqMax = 2):

  Slot_Memory = 2 × 6.4 GB = ~12.8 GB

Step 4 — Total VRAM:

  Total_VRAM = 36.0 GB + 12.8 GB = ~48.8 GB`;

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
  }, [modelUrl, contextWindow, bytesPerElement, slots, token]);

  useEffect(() => {
    if (hasCalculated.current && modelUrl.trim()) {
      performCalculation(false);
    }
  }, [contextWindow, bytesPerElement, slots]);

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
                  <label htmlFor="modelUrl">                    
                    Ex. Qwen/Qwen3-8B-GGUF/Qwen3-8B-Q8_0.gguf<br />
                    Ex. https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf<br/><br/>
                    Model URL (download link or org/family/file)
                  </label>
          <input
            id="modelUrl"
            type="text"
            value={modelUrl}
            onChange={(e) => setModelUrl(e.target.value)}
            placeholder="https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf"
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
              <span className="vram-result-label">KV Per Token Per Layer</span>
              <span className="vram-result-value">{formatBytes(result.kv_per_token_per_layer)}</span>
            </div>
            <div className="vram-result-item">
              <span className="vram-result-label">Context Window (Used)</span>
              <span className="vram-result-value">{result.input.context_window.toLocaleString()} tokens</span>
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
