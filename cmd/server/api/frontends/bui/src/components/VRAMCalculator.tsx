import { useState, useRef, useCallback } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { VRAMCalculatorResponse } from '../types';
import { VRAMCalculatorPanel, useVRAMState } from './vram';

export default function VRAMCalculator() {
  const { token } = useToken();
  const [modelUrl, setModelUrl] = useState('');
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<VRAMCalculatorResponse | null>(null);
  const [calculatedModelUrl, setCalculatedModelUrl] = useState('');
  const cachedKeyRef = useRef('');

  const { controlsProps, resultsProps } = useVRAMState({ serverResponse: result, enableHardwareOverrides: true });

  const performCalculation = useCallback(async () => {
    const trimmed = modelUrl.trim();
    if (!trimmed) {
      setError('Please enter a model URL');
      return;
    }

    const cacheKey = [
      trimmed,
      controlsProps.contextWindow,
      controlsProps.bytesPerElement,
      controlsProps.slots,
      controlsProps.gpuLayers,
      controlsProps.expertLayersOnGPU,
      controlsProps.kvCacheOnCPU,
      controlsProps.deviceCount,
      controlsProps.tensorSplit,
    ].join('|');
    if (cacheKey === cachedKeyRef.current && result) {
      return;
    }

    setLoading(true);
    setError(null);
    setResult(null);

    try {
      const response = await api.calculateVRAM(
        {
          model_url: trimmed,
          context_window: controlsProps.contextWindow,
          bytes_per_element: controlsProps.bytesPerElement,
          slots: controlsProps.slots,
        },
        token || undefined
      );
      setResult(response);
      setCalculatedModelUrl(trimmed);
      cachedKeyRef.current = cacheKey;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to calculate VRAM');
      cachedKeyRef.current = '';
    } finally {
      setLoading(false);
    }
  }, [modelUrl, controlsProps, token, result]);

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
        <a
          href="https://www.kronkai.com/blog/understanding-the-kronk-vram-calculator"
          target="_blank"
          rel="noopener noreferrer"
          className="btn btn-secondary"
        >
          Learn More
        </a>
      </div>

      <form onSubmit={handleCalculate} className="form-card">
        <div className="form-group">
                  <label htmlFor="modelUrl">                    
                    Ex. <code>bartowski/Qwen3-8B-GGUF:Q4_K_M</code> (shorthand)<br/>
                    Ex. <code>https://huggingface.co/Qwen/Qwen3-8B-GGUF/resolve/main/Qwen3-8B-Q8_0.gguf</code><br/>
                    Ex. <code>https://huggingface.co/unsloth/Qwen3.5-35B-A3B-GGUF/resolve/main/Qwen3.5-35B-A3B-UD-Q8_K_XL.gguf</code><br/><br/>
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
          resultsProps={resultsProps}
          variant="form"
          hideResults
        />

        <div style={{ display: 'flex', justifyContent: 'flex-end', marginTop: '16px' }}>
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
          modelUrl={calculatedModelUrl}
        />
      )}
    </div>
  );
}
