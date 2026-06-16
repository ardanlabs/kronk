import { useState } from 'react';
import { api } from '../services/api';
import { useDownload } from '../contexts/DownloadContext';
import DownloadProgressBar from './DownloadProgressBar';
import type { ResolveSourceResponse, HFRepoFile } from '../types';
import {
  stripGGUF,
  modelIDFromFilename,
  isQuantOnly,
  isMMProjFile,
  matchesQuant,
  splitPaste,
  groupRepoFiles,
} from '../lib/hf';

// buildSource composes the source string sent to /v1/catalog/resolve
// from the three fields. When Model is empty, the server returns the
// repo file list so the user can pick. When all three are filled, the
// owner/repo/file shorthand routes through hf.ParseInput.
function buildSource(provider: string, family: string, model: string): string {
  const p = provider.trim();
  const f = family.trim();
  const m = stripGGUF(model.trim());

  if (!p || !f) return '';
  if (!m) return `${p}/${f}`;
  return `${p}/${f}/${m}.gguf`;
}

export default function ModelPull() {
  const { download, isDownloading, startDownload, cancelDownload, clearDownload } = useDownload();

  const [provider, setProvider] = useState('');
  const [family, setFamily] = useState('');
  const [model, setModel] = useState('');

  const [resolved, setResolved] = useState<ResolveSourceResponse | null>(null);
  const [repoFiles, setRepoFiles] = useState<HFRepoFile[] | null>(null);
  const [resolveError, setResolveError] = useState<string | null>(null);
  const [isResolving, setIsResolving] = useState(false);

  const [showOverride, setShowOverride] = useState(false);
  const [projOverride, setProjOverride] = useState('');

  const [showMTPOverride, setShowMTPOverride] = useState(false);
  const [mtpOverride, setMtpOverride] = useState('');

  const isComplete = download?.status === 'complete';
  const hasError = download?.status === 'error';

  const canResolve = provider.trim().length > 0 && family.trim().length > 0;

  // runResolve dispatches to one of two endpoints based on whether the
  // Model field is filled:
  //
  //   - Model blank → /v1/catalog/lookup with "provider/family". Returns
  //     every GGUF in the repo so the user can pick one. This is the
  //     "browse" path.
  //   - Model filled → /v1/catalog/resolve with the 3-segment shorthand
  //     "provider/family/model.gguf". Returns the canonical resolution
  //     (download URLs, projection, cache flags) for preview before pull.
  //
  // The server cannot reliably tell "owner/repo" from "owner/modelID"
  // (both are one-slash strings), so the BUI picks the right endpoint.
  const runResolve = async (modelOverride?: string) => {
    if (isResolving || isDownloading) return;

    const p = provider.trim();
    const f = family.trim();
    const m = stripGGUF((modelOverride ?? model).trim());

    if (!p || !f) {
      setResolveError('Provider and Family are required');
      return;
    }

    setIsResolving(true);
    setResolveError(null);
    setResolved(null);
    setRepoFiles(null);

    try {
      if (!m) {
        const lookup = await api.lookupHuggingFace(`${p}/${f}`);
        setRepoFiles(lookup.repo_files ?? []);
        return;
      }

      // Quant-only shortcut: the user typed "Q4_K_M" rather than the
      // full file basename. Look up the repo, filter to non-mmproj files
      // whose basename ends in that quant. One match → resolve directly.
      // Multiple matches (e.g. UD- and non-UD variants) → show picker.
      if (isQuantOnly(m)) {
        const lookup = await api.lookupHuggingFace(`${p}/${f}`);
        const matches = (lookup.repo_files ?? []).filter(
          (file) => !isMMProjFile(file.filename) && matchesQuant(file.filename, m),
        );

        if (matches.length === 0) {
          setResolveError(`No GGUF file matching quant "${m}" found in ${p}/${f}`);
          return;
        }

        // Deduplicate split shards: every shard maps to the same model id.
        const uniqueIDs = new Set(matches.map((file) => modelIDFromFilename(file.filename)));

        if (uniqueIDs.size > 1) {
          setRepoFiles(matches);
          return;
        }

        const id = [...uniqueIDs][0];
        setModel(id);
        const res = await api.resolveSource(`${p}/${f}/${id}.gguf`);
        setResolved(res);
        return;
      }

      const res = await api.resolveSource(`${p}/${f}/${m}.gguf`);
      setResolved(res);
    } catch (err) {
      setResolveError(err instanceof Error ? err.message : String(err));
    } finally {
      setIsResolving(false);
    }
  };

  const handleResolve = () => void runResolve();

  const handlePickFile = (filename: string) => {
    const id = modelIDFromFilename(filename);
    setModel(id);
    setRepoFiles(null);
    // Re-resolve immediately so the user lands on the preview card.
    void runResolve(id);
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
      handleResolve();
    }
  };

  const handleClearResolve = () => {
    setResolved(null);
    setRepoFiles(null);
    setResolveError(null);
    setProjOverride('');
    setShowOverride(false);
    setMtpOverride('');
    setShowMTPOverride(false);
  };

  const handlePull = () => {
    if (!resolved || isDownloading || resolved.installed) return;

    // Both companion URLs are sent ONLY as explicit overrides. Left empty,
    // the server resolves the model id and downloads every companion it
    // needs (projection AND mtp drafter). Auto-sending a resolved companion
    // URL here would flip the server into explicit-URL mode, where any
    // companion whose URL was not also supplied (e.g. the projection) is
    // silently skipped. Keep proj and mtp symmetric: override-only.
    const proj = showOverride ? projOverride.trim() : '';
    const mtp = showMTPOverride ? mtpOverride.trim() : '';
    // The server handles id → URL resolution, so the BUI can always send
    // the canonical id regardless of mode.
    const modelArg = resolved.canonical_id || buildSource(provider, family, model);

    startDownload(modelArg, proj || undefined, mtp || undefined);
  };

  const sourceLabel = resolved?.from_local
    ? 'on disk'
    : resolved?.from_cache
      ? 'cached'
      : 'fetched from network';

  return (
    <div>
      <div className="page-header">
        <h2>HF Pull GGUF Model</h2>
        <p>
          Identify the model with three fields. Each one maps to a segment of the HuggingFace
          file URL:
        </p>

        {/*
          Layout uses fixed character positions inside a <pre> so the
          underline brackets and labels line up with the URL segments
          above them. Counts (0-indexed):
            "https://huggingface.co/" → 23 chars
            "unsloth"                 → 7 chars   (positions 23-29)
            "/"                       → 1 char    (position  30)
            "Qwen3.6-27B-GGUF"        → 16 chars  (positions 31-46)
            "/blob/main/"             → 11 chars  (positions 47-57)
            "Qwen3.6-27B-Q4_K_M"      → 18 chars  (positions 58-75)
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
          {/* 23 spaces, then 7-wide bracket, 1 space, 16-wide bracket, 11 spaces, 18-wide bracket */}
          {'                       '}
          <span style={{ color: 'var(--accent, #60a5fa)' }}>└─────┘</span>
          {' '}
          <span style={{ color: 'var(--success, #4ade80)' }}>└──────────────┘</span>
          {'           '}
          <span style={{ color: 'var(--warning, #fbbf24)' }}>└────────────────┘</span>
          {'\n'}
          {/* labels centered under each segment:
                Provider centered on col 26 (segment cols 23-29) → starts col 22
                Family   centered on col 39 (segment cols 31-46) → starts col 36
                Model    centered on col 66 (segment cols 58-75) → starts col 64 */}
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
            disabled={isResolving || isDownloading}
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
            disabled={isResolving || isDownloading}
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
            disabled={isResolving || isDownloading}
          />
        </div>

        <div style={{ display: 'flex', gap: '8px' }}>
          <button
            type="button"
            className="btn btn-secondary"
            onClick={handleResolve}
            disabled={isResolving || isDownloading || !canResolve}
          >
            {isResolving ? 'Resolving…' : model.trim() ? 'Resolve' : 'Browse files'}
          </button>
          {(resolved || repoFiles || resolveError) && !isDownloading && (
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

        {resolveError && (
          <div className="status-box">
            <div className="status-line error">{resolveError}</div>
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
                            disabled={isResolving || isDownloading}
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
                <tr>
                  <td>MTP drafter</td>
                  <td>
                    {resolved.download_mtp
                      ? <code style={{ wordBreak: 'break-all' }}>{resolved.download_mtp}</code>
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

            <details
              style={{ marginTop: '12px' }}
              open={showMTPOverride}
              onToggle={(e) => setShowMTPOverride((e.target as HTMLDetailsElement).open)}
            >
              <summary style={{ cursor: 'pointer', userSelect: 'none' }}>
                Override MTP drafter URL
              </summary>
              <div className="form-group" style={{ marginTop: '8px' }}>
                <label htmlFor="mtpOverride">MTP drafter URL (fully qualified HuggingFace URL)</label>
                <input
                  type="text"
                  id="mtpOverride"
                  value={mtpOverride}
                  onChange={(e) => setMtpOverride(e.target.value)}
                  placeholder="https://huggingface.co/org/repo/resolve/main/mtp-model.gguf"
                  disabled={isDownloading}
                />
                <p style={{ fontSize: '12px', opacity: 0.7, margin: '4px 0 0 0' }}>
                  When set, the explicit MTP drafter URL replaces the resolver's choice.
                  Leave the field empty (or close this section) to use the MTP drafter above.
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
