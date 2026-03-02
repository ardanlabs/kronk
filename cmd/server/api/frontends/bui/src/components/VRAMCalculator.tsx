import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { VRAMCalculatorResponse } from '../types';
import { VRAMFormulaModal, VRAMControls, VRAMResults } from './vram';

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

      {showLearnMore && <VRAMFormulaModal onClose={() => setShowLearnMore(false)} />}

      <form onSubmit={handleCalculate} className="form-card">
        <div className="form-group">
                  <label htmlFor="modelUrl">                    
                    Ex. https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf<br/>
                    Ex. https://huggingface.co/unsloth/Qwen3-Coder-Next-GGUF/tree/main/UD-Q5_K_XL (split models)<br/><br/>
                    Model URL (download link, org/family/file, or folder for split models)
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
            Enter a HuggingFace URL to a GGUF model file, or a folder URL for split models
          </small>
        </div>

        <VRAMControls
          contextWindow={contextWindow}
          onContextWindowChange={setContextWindow}
          bytesPerElement={bytesPerElement}
          onBytesPerElementChange={setBytesPerElement}
          slots={slots}
          onSlotsChange={setSlots}
          variant="form"
        />

        <button type="submit" className="btn btn-primary" disabled={loading}>
          {loading ? 'Calculating...' : 'Calculate VRAM'}
        </button>
      </form>

      {error && <div className="alert alert-error">{error}</div>}

      {result && (
        <VRAMResults
          totalVram={result.total_vram}
          slotMemory={result.slot_memory}
          kvPerSlot={result.kv_per_slot}
          kvPerTokenPerLayer={result.kv_per_token_per_layer}
          input={result.input}
        />
      )}
    </div>
  );
}
