import { useState, useRef, useCallback } from 'react';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { VRAMCalculatorResponse, HFRepoFile } from '../types';
import { VRAMCalculatorPanel, useVRAMState } from './vram';
import { EXPERTS_ALL_ON_GPU } from './vram/constants';
import {
  stripGGUF,
  modelIDFromFilename,
  isQuantOnly,
  isMMProjFile,
  matchesQuant,
  splitPaste,
  groupRepoFiles,
  isSplitFilename,
} from '../lib/hf';

// buildModelURL composes the HuggingFace URL the VRAM endpoint accepts.
// For split shards the calculator wants the folder URL so the server
// sums every shard in the group; for everything else it wants the
// single-file resolve URL.
function buildModelURL(provider: string, family: string, filename: string): string {
  const p = provider.trim();
  const f = family.trim();
  if (isSplitFilename(filename)) {
    const slashIdx = filename.lastIndexOf('/');
    const folder = slashIdx >= 0 ? filename.slice(0, slashIdx) : '';
    if (folder) {
      return `https://huggingface.co/${p}/${f}/tree/main/${folder}`;
    }
    return `https://huggingface.co/${p}/${f}/tree/main`;
  }
  return `https://huggingface.co/${p}/${f}/resolve/main/${filename}`;
}

export default function VRAMCalculator() {
  const { token } = useToken();

  const [provider, setProvider] = useState('');
  const [family, setFamily] = useState('');
  const [model, setModel] = useState('');

  const [repoFiles, setRepoFiles] = useState<HFRepoFile[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const [isResolving, setIsResolving] = useState(false);
  const [loading, setLoading] = useState(false);

  const [result, setResult] = useState<VRAMCalculatorResponse | null>(null);
  const [calculatedModelLabel, setCalculatedModelLabel] = useState('');
  const cachedKeyRef = useRef('');

  const { controlsProps, resultsProps } = useVRAMState({
    serverResponse: result,
    enableHardwareOverrides: true,
    modelUrl: calculatedModelLabel || undefined,
    authToken: token || undefined,
  });

  const canResolve = provider.trim().length > 0 && family.trim().length > 0;

  // runCalculate handles all of:
  //
  //   - Model blank             → browse files (show picker)
  //   - Model is a quant tag    → lookup repo, match the quant
  //   - Model is a full basename → try local model id first, then HF
  //
  // The Model-filled paths end by invoking calculateOne() with the
  // selected filename so a single code path performs the final HF
  // calculateVRAM call.
  const runCalculate = useCallback(async (modelOverride?: string) => {
    if (isResolving || loading) return;

    const p = provider.trim();
    const f = family.trim();
    const m = stripGGUF((modelOverride ?? model).trim());

    if (!p || !f) {
      setError('Provider and Family are required');
      return;
    }

    setError(null);
    setResult(null);
    setRepoFiles(null);
    cachedKeyRef.current = '';

    // Model blank → browse the repo so the user can pick a file.
    if (!m) {
      setIsResolving(true);
      try {
        const lookup = await api.lookupHuggingFace(`${p}/${f}`);
        setRepoFiles(lookup.repo_files ?? []);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setIsResolving(false);
      }
      return;
    }

    // Quant-only shortcut: lookup the repo and pick the matching file.
    if (isQuantOnly(m)) {
      setIsResolving(true);
      try {
        const lookup = await api.lookupHuggingFace(`${p}/${f}`);
        const matches = (lookup.repo_files ?? []).filter(
          (file) => !isMMProjFile(file.filename) && matchesQuant(file.filename, m),
        );

        if (matches.length === 0) {
          setError(`No GGUF file matching quant "${m}" found in ${p}/${f}`);
          return;
        }

        // Deduplicate split shards: every shard maps to the same model id.
        const uniqueIDs = new Set(matches.map((file) => modelIDFromFilename(file.filename)));

        if (uniqueIDs.size > 1) {
          setRepoFiles(matches);
          return;
        }

        const pick = matches[0];
        setModel(modelIDFromFilename(pick.filename));
        await calculateOne(pick.filename);
      } catch (err) {
        setError(err instanceof Error ? err.message : String(err));
      } finally {
        setIsResolving(false);
      }
      return;
    }

    // Model is a full basename. Try the local model id first so users
    // who already have the model installed get an instant answer with
    // no HuggingFace round trip; fall back to HF on not-found.
    setLoading(true);
    const localID = `${p}/${m}`;
    try {
      const response = await api.calculateVRAM(
        {
          model_id: localID,
          context_window: controlsProps.contextWindow,
          bytes_per_element: controlsProps.bytesPerElement,
          slots: controlsProps.slots,
          expert_layers_on_gpu: EXPERTS_ALL_ON_GPU,
        },
        token || undefined,
      );
      setResult(response);
      setCalculatedModelLabel(localID);
      setLoading(false);
      return;
    } catch (err) {
      const msg = err instanceof Error ? err.message : '';
      if (!/not found|no such model|404/i.test(msg)) {
        setError(msg);
        setLoading(false);
        return;
      }
      // Not installed locally — fall through to HF.
    } finally {
      setLoading(false);
    }

    await calculateOne(`${m}.gguf`);
  }, [isResolving, loading, provider, family, model, controlsProps, token]);

  // calculateOne resolves a specific filename to the HF URL the
  // calculateVRAM endpoint understands and stores the result. Split
  // shards collapse to the folder URL so the server sums every shard.
  const calculateOne = useCallback(async (filename: string) => {
    const p = provider.trim();
    const f = family.trim();
    if (!p || !f) {
      setError('Provider and Family are required');
      return;
    }

    const url = buildModelURL(p, f, filename);
    const label = `${p}/${f}/${filename}`;
    const cacheKey = [
      url,
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

    try {
      const response = await api.calculateVRAM(
        {
          model_url: url,
          context_window: controlsProps.contextWindow,
          bytes_per_element: controlsProps.bytesPerElement,
          slots: controlsProps.slots,
          expert_layers_on_gpu: EXPERTS_ALL_ON_GPU,
        },
        token || undefined,
      );
      setResult(response);
      setCalculatedModelLabel(label);
      cachedKeyRef.current = cacheKey;
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to calculate VRAM');
      cachedKeyRef.current = '';
    } finally {
      setLoading(false);
    }
  }, [provider, family, controlsProps, token, result]);

  const handlePickFile = (filename: string) => {
    setModel(modelIDFromFilename(filename));
    setRepoFiles(null);
    void calculateOne(filename);
  };

  const handleProviderPaste = (e: React.ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text');
    const split = splitPaste(text);
    if (!split) return;

    e.preventDefault();
    setProvider(split.provider);
    setFamily(split.family);
    setModel(split.model);
  };

  const handleFieldKey = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' && canResolve) {
      e.preventDefault();
      void runCalculate();
    }
  };

  const handleClear = () => {
    setRepoFiles(null);
    setError(null);
    setResult(null);
    setCalculatedModelLabel('');
    cachedKeyRef.current = '';
  };

  const handleCalculateClick = () => {
    void runCalculate();
  };

  return (
    <div>
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

      <div className="page-header">
        <p>
          Identify the model with three fields. Each one maps to a segment of the HuggingFace
          file URL:
        </p>

        {/*
          Layout uses fixed character positions inside a <pre> so the
          underline brackets and labels line up with the URL segments
          above them.
        */}
        <pre
          style={{
            fontSize: '14px',
            lineHeight: '1.4',
            padding: '12px 14px',
            background: 'var(--bg-2, #1a1a1a)',
            border: '1px solid var(--border, #333)',
            borderRadius: '4px',
            margin: '8px 0',
            overflowX: 'auto',
            fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace',
            color: 'var(--text, #e5e5e5)',
          }}
        >
          <span style={{ opacity: 0.85 }}>https://huggingface.co/</span>
          <span style={{ color: 'var(--accent, #60a5fa)', fontWeight: 600 }}>unsloth</span>
          <span style={{ opacity: 0.85 }}>/</span>
          <span style={{ color: 'var(--success, #4ade80)', fontWeight: 600 }}>Qwen3.6-27B-GGUF</span>
          <span style={{ opacity: 0.85 }}>/blob/main/</span>
          <span style={{ color: 'var(--warning, #fbbf24)', fontWeight: 600 }}>Qwen3.6-27B-Q4_K_M</span>
          <span style={{ opacity: 0.85 }}>.gguf</span>
          {'\n'}
          {'                       '}
          <span style={{ color: 'var(--accent, #60a5fa)' }}>└─────┘</span>
          {' '}
          <span style={{ color: 'var(--success, #4ade80)' }}>└──────────────┘</span>
          {'           '}
          <span style={{ color: 'var(--warning, #fbbf24)' }}>└────────────────┘</span>
          {'\n'}
          {'                      '}
          <span style={{ color: 'var(--accent, #60a5fa)', fontWeight: 600 }}>Provider</span>
          {'      '}
          <span style={{ color: 'var(--success, #4ade80)', fontWeight: 600 }}>Family</span>
          {'                      '}
          <span style={{ color: 'var(--warning, #fbbf24)', fontWeight: 600 }}>Model</span>
        </pre>

        <ul style={{ margin: '4px 0 0 0', paddingLeft: '20px', fontSize: '13px' }}>
          <li>
            <strong>Model is optional.</strong> Leave it blank and click <em>Browse files</em> to
            see every GGUF in the repo and pick one.
          </li>
          <li>
            <strong>Quant shortcut.</strong> The Model field also accepts just a quant tag
            (e.g. <code>Q4_K_M</code>, <code>Q8_0</code>, <code>BF16</code>) — we'll find the
            matching file in the repo for you.
          </li>
          <li>
            <strong>Local models.</strong> When the full basename is filled and the model is
            already installed, the calculator uses the on-disk file and skips HuggingFace.
          </li>
          <li>
            <strong>Paste anything.</strong> Pasting a full HuggingFace URL or{' '}
            <code>owner/repo[/file.gguf]</code> shorthand into the Provider field auto-splits
            it across all three fields.
          </li>
        </ul>
      </div>

      <div className="card">
        <div className="form-group">
          <label htmlFor="provider">Provider <span style={{ opacity: 0.6 }}>(required)</span></label>
          <input
            type="text"
            id="provider"
            value={provider}
            onChange={(e) => setProvider(e.target.value)}
            onPaste={handleProviderPaste}
            onKeyDown={handleFieldKey}
            placeholder="unsloth"
            disabled={isResolving || loading}
          />
        </div>

        <div className="form-group">
          <label htmlFor="family">Family <span style={{ opacity: 0.6 }}>(required)</span></label>
          <input
            type="text"
            id="family"
            value={family}
            onChange={(e) => setFamily(e.target.value)}
            onKeyDown={handleFieldKey}
            placeholder="Qwen3-0.6B-GGUF"
            disabled={isResolving || loading}
          />
        </div>

        <div className="form-group">
          <label htmlFor="model">
            Model <span style={{ opacity: 0.6 }}>(optional — full basename, just a quant tag, or blank)</span>
          </label>
          <input
            type="text"
            id="model"
            value={model}
            onChange={(e) => setModel(e.target.value)}
            onKeyDown={handleFieldKey}
            placeholder="Qwen3-0.6B-Q8_0   ·   Q4_K_M   ·   (blank)"
            disabled={isResolving || loading}
          />
        </div>

        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            type="button"
            className="btn btn-primary"
            onClick={handleCalculateClick}
            disabled={isResolving || loading || !canResolve}
          >
            {isResolving
              ? 'Looking up…'
              : loading
                ? 'Calculating…'
                : model.trim()
                  ? 'Calculate VRAM'
                  : 'Browse files'}
          </button>
          {(repoFiles || result || error) && !loading && !isResolving && (
            <button
              type="button"
              className="btn"
              onClick={handleClear}
              disabled={isResolving}
            >
              Clear
            </button>
          )}
        </div>

        {error && (
          <div className="status-box">
            <div className="status-line error">{error}</div>
          </div>
        )}

        {repoFiles && (() => {
          const rows = groupRepoFiles(repoFiles);
          return (
            <div className="card" style={{ background: 'var(--bg-2, #1a1a1a)', marginTop: '12px' }}>
              <div style={{ marginBottom: '12px' }}>
                <strong>Pick a file from </strong>
                <code>{provider.trim()}/{family.trim()}</code>
                <span style={{ fontSize: '12px', opacity: 0.7, marginLeft: '8px' }}>
                  ({rows.length} GGUF model{rows.length === 1 ? '' : 's'})
                </span>
              </div>
              {rows.length === 0 ? (
                <div style={{ opacity: 0.7 }}>No GGUF files found in this repository.</div>
              ) : (
                <table className="kv-table">
                  <thead>
                    <tr><th style={{ textAlign: 'left' }}>Filename</th><th>Size</th><th></th></tr>
                  </thead>
                  <tbody>
                    {rows.map((r) => (
                      <tr key={r.label}>
                        <td>
                          <code style={{ wordBreak: 'break-all' }}>{r.label}</code>
                          {r.parts > 1 && (
                            <span style={{ fontSize: '11px', opacity: 0.7, marginLeft: '8px' }}>
                              ({r.parts} shards)
                            </span>
                          )}
                        </td>
                        <td style={{ whiteSpace: 'nowrap' }}>{r.sizeStr}</td>
                        <td>
                          <button
                            type="button"
                            className="btn btn-secondary"
                            onClick={() => handlePickFile(r.filename)}
                            disabled={isResolving || loading}
                          >
                            Select
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </div>
          );
        })()}

        <div style={{ marginTop: '16px' }}>
          <VRAMCalculatorPanel
            controlsProps={controlsProps}
            resultsProps={resultsProps}
            variant="form"
            hideResults
          />
        </div>
      </div>

      {(loading || isResolving) && (
        <div className="vram-loading-banner">
          <span className="vram-loading-spinner" />
          <span>{isResolving ? 'Looking up repository…' : 'Fetching model header (up to 16 MB)…'}</span>
        </div>
      )}

      {resultsProps && (
        <VRAMCalculatorPanel
          controlsProps={controlsProps}
          resultsProps={resultsProps}
          hideControls
          modelUrl={calculatedModelLabel}
        />
      )}
    </div>
  );
}
