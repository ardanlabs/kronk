import { useState } from 'react';
import { api } from '../services/api';
import { useDownload } from '../contexts/DownloadContext';
import DownloadInfoTable from './DownloadInfoTable';
import DownloadProgressBar from './DownloadProgressBar';
import type { ResolveSourceResponse } from '../types';

export default function ModelPull() {
  const { download, isDownloading, startDownload, cancelDownload, clearDownload } = useDownload();

  const [source, setSource] = useState('');
  const [resolved, setResolved] = useState<ResolveSourceResponse | null>(null);
  const [resolveError, setResolveError] = useState<string | null>(null);
  const [isResolving, setIsResolving] = useState(false);

  const [showOverride, setShowOverride] = useState(false);
  const [projOverride, setProjOverride] = useState('');

  const isComplete = download?.status === 'complete';
  const hasError = download?.status === 'error';

  const handleResolve = async () => {
    const trimmed = source.trim();
    if (!trimmed || isResolving || isDownloading) return;

    setIsResolving(true);
    setResolveError(null);
    setResolved(null);

    try {
      const res = await api.resolveSource(trimmed);
      setResolved(res);
    } catch (err) {
      setResolveError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsResolving(false);
    }
  };

  const handleSourceKey = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault();
      void handleResolve();
    }
  };

  const handleClearResolve = () => {
    setResolved(null);
    setResolveError(null);
    setProjOverride('');
    setShowOverride(false);
  };

  const handlePull = () => {
    if (!resolved || isDownloading || resolved.installed) return;

    // The /v1/models/pull endpoint accepts the original source string
    // and re-runs the resolver itself. Sending the canonical id keeps
    // the request small and avoids any mismatch with what was just
    // resolved on screen.
    const modelArg = resolved.canonical_id || source.trim();

    const proj = showOverride ? projOverride.trim() : '';
    startDownload(modelArg, proj || undefined);
  };

  const sourceLabel = `${resolved?.from_local ? 'on disk' : resolved?.from_cache ? 'cached' : 'fetched from network'}`;

  return (
    <div>
      <div className="page-header">
        <h2>Pull Model</h2>
        <p>Download a new model from HuggingFace. The resolver finds the canonical URL(s) and projection file for you — splits and vision/audio companions are handled automatically.</p>
        <p>Source forms accepted:</p>
        <ul style={{ margin: '4px 0 0 0', paddingLeft: '20px' }}>
          <li>Bare id: <code>Qwen3-0.6B-Q8_0</code></li>
          <li>Canonical id: <code>unsloth/Qwen3-0.6B-Q8_0</code></li>
          <li>Shorthand: <code>bartowski/Qwen3-8B-GGUF:Q4_K_M</code> (or with <code>hf.co/</code> prefix or <code>@revision</code>)</li>
          <li>Full URL: <code>https://huggingface.co/org/repo/resolve/main/model.gguf</code></li>
        </ul>
      </div>

      <div className="card">
        <div className="form-group">
          <label htmlFor="source">Source</label>
          <div style={{ display: 'flex', gap: '8px' }}>
            <input
              type="text"
              id="source"
              value={source}
              onChange={(e) => setSource(e.target.value)}
              onKeyDown={handleSourceKey}
              placeholder="owner/repo:Q4_K_M  ·  unsloth/Qwen3-8B-Q4_K_M  ·  https://huggingface.co/.../model.gguf"
              disabled={isResolving || isDownloading}
              style={{ flex: 1 }}
            />
            <button
              type="button"
              className="btn btn-secondary"
              onClick={handleResolve}
              disabled={isResolving || isDownloading || source.trim().length === 0}
            >
              {isResolving ? 'Resolving…' : 'Resolve'}
            </button>
            {(resolved || resolveError) && !isDownloading && (
              <button
                type="button"
                className="btn"
                onClick={handleClearResolve}
                disabled={isResolving}
              >
                Clear
              </button>
            )}
          </div>
        </div>

        {resolveError && (
          <div className="status-box">
            <div className="status-line error">{resolveError}</div>
          </div>
        )}

        {resolved && (
          <div className="card" style={{ background: 'var(--bg-2, #1a1a1a)', marginTop: '12px' }}>
            <div style={{ display: 'flex', alignItems: 'baseline', gap: '12px', marginBottom: '12px' }}>
              <strong style={{ fontSize: '16px' }}>{resolved.canonical_id}</strong>
              <span style={{ fontSize: '12px', opacity: 0.7 }}>({sourceLabel})</span>
              {resolved.installed && (
                <span style={{ fontSize: '12px', color: 'var(--success, #4ade80)' }}>● already installed</span>
              )}
            </div>

            <table className="kv-table">
              <tbody>
                <tr><td>Provider</td><td><code>{resolved.provider}</code></td></tr>
                <tr><td>Family</td><td><code>{resolved.family}</code></td></tr>
                <tr><td>Revision</td><td><code>{resolved.revision || 'main'}</code></td></tr>
                <tr>
                  <td>Files{resolved.download_urls.length > 1 ? ` (${resolved.download_urls.length} shards)` : ''}</td>
                  <td>
                    {resolved.download_urls.map((u, i) => (
                      <div key={i}><code style={{ wordBreak: 'break-all' }}>{u}</code></div>
                    ))}
                  </td>
                </tr>
                <tr>
                  <td>Projection</td>
                  <td>
                    {resolved.download_proj
                      ? <code style={{ wordBreak: 'break-all' }}>{resolved.download_proj}</code>
                      : <span style={{ opacity: 0.6 }}>none</span>}
                  </td>
                </tr>
              </tbody>
            </table>

            <details
              style={{ marginTop: '12px' }}
              open={showOverride}
              onToggle={(e) => setShowOverride((e.target as HTMLDetailsElement).open)}
            >
              <summary style={{ cursor: 'pointer', userSelect: 'none' }}>
                Override projection URL
              </summary>
              <div className="form-group" style={{ marginTop: '8px' }}>
                <label htmlFor="projOverride">Projection URL (fully qualified HuggingFace URL)</label>
                <input
                  type="text"
                  id="projOverride"
                  value={projOverride}
                  onChange={(e) => setProjOverride(e.target.value)}
                  placeholder="https://huggingface.co/org/repo/resolve/main/mmproj-model.gguf"
                  disabled={isDownloading}
                />
                <p style={{ fontSize: '12px', opacity: 0.7, margin: '4px 0 0 0' }}>
                  When set, the explicit projection URL replaces the resolver's choice.
                  Leave the field empty (or close this section) to use the projection above.
                </p>
              </div>
            </details>

            <div style={{ display: 'flex', gap: '12px', marginTop: '16px' }}>
              <button
                type="button"
                className="btn btn-primary"
                onClick={handlePull}
                disabled={isDownloading || resolved.installed}
                title={resolved.installed ? 'Model is already installed' : ''}
              >
                {isDownloading ? 'Downloading…' : 'Pull'}
              </button>
              {isDownloading && (
                <button className="btn btn-danger" type="button" onClick={cancelDownload}>
                  Cancel
                </button>
              )}
              {(isComplete || hasError) && (
                <button className="btn" type="button" onClick={clearDownload}>
                  Clear progress
                </button>
              )}
            </div>
          </div>
        )}

        {download && download.meta && (
          <DownloadInfoTable meta={download.meta} />
        )}

        {download && download.progress && isDownloading && (
          <DownloadProgressBar progress={download.progress} meta={download.meta} />
        )}

        {download && download.messages.length > 0 && (
          <div className="status-box">
            {download.messages.map((msg, idx) => (
              <div key={idx} className={`status-line ${msg.type}`}>
                {msg.text}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
