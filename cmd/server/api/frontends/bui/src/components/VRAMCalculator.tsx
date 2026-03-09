import { useState, useEffect, useRef, useCallback } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { VRAMCalculatorResponse } from '../types';
import { VRAMFormulaModal, VRAMCalculatorPanel, useVRAMState } from './vram';

export default function VRAMCalculator() {
  const { token } = useToken();
  const [modelUrl, setModelUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<VRAMCalculatorResponse | null>(null);
  const [showLearnMore, setShowLearnMore] = useState(false);
  const hasCalculated = useRef(false);

  const { controlsProps, resultsProps } = useVRAMState({ serverResponse: result });

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
          context_window: controlsProps.contextWindow,
          bytes_per_element: controlsProps.bytesPerElement,
          slots: controlsProps.slots,
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
  }, [modelUrl, controlsProps.contextWindow, controlsProps.bytesPerElement, controlsProps.slots, token]);

  useEffect(() => {
    if (hasCalculated.current && modelUrl.trim()) {
      performCalculation(false);
    }
  }, [controlsProps.contextWindow, controlsProps.bytesPerElement, controlsProps.slots]);

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
                    Ex. <code>bartowski/Qwen3-8B-GGUF:Q4_K_M</code> (shorthand)<br/>
                    Ex. <code>https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf</code><br/>
                    Ex. <code>https://huggingface.co/unsloth/Qwen3-Coder-Next-GGUF/tree/main/UD-Q5_K_XL</code> (split models)<br/><br/>
                    Model URL, shorthand, or folder for split models
                  </label>
          <input
            id="modelUrl"
            type="text"
            value={modelUrl}
            onChange={(e) => setModelUrl(e.target.value)}
            placeholder="bartowski/Qwen3-8B-GGUF:Q4_K_M"
            className="form-input"
          />
          <small className="form-hint">
            Enter a shorthand (owner/repo:TAG), full HuggingFace URL, or folder URL for split models
          </small>
        </div>

        <VRAMCalculatorPanel
          controlsProps={controlsProps}
          variant="form"
        />

        <div style={{ display: 'flex', justifyContent: 'flex-end' }}>
          <button type="submit" className="btn btn-primary" disabled={loading}>
            {loading ? 'Calculating...' : 'Calculate VRAM'}
          </button>
        </div>
      </form>

      {loading && (
        <div className="vram-loading-banner">
          <span className="vram-loading-spinner" />
          <span>Fetching model header (up to 16 MB)…</span>
        </div>
      )}

      {error && <div className="alert alert-error">{error}</div>}

      {resultsProps && (
        <VRAMCalculatorPanel
          controlsProps={controlsProps}
          resultsProps={resultsProps}
          hideControls
        />
      )}
    </div>
  );
}
