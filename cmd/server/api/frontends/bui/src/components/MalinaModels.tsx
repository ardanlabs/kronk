import { Fragment, useCallback, useEffect, useMemo, useState } from 'react';
import { api } from '../services/api';
import type { MalinaCatalogEntry, MalinaCatalogFile, MalinaModelEntry } from '../types';
import { useDownload } from '../contexts/DownloadContext';

type SortColumn = 'name' | 'size';
type SortDir = 'asc' | 'desc';

function parseSizeMB(size: string): number {
  const match = /^([\d.]+)\s*(MB|GB|KB|B)?$/i.exec(size.trim());
  if (!match) return 0;
  const value = Number.parseFloat(match[1]);
  switch ((match[2] || 'MB').toUpperCase()) {
    case 'GB': return value * 1000;
    case 'KB': return value / 1000;
    case 'B': return value / 1_000_000;
    default: return value;
  }
}

function bundleSizeMB(files: MalinaCatalogFile[]): number {
  return files.reduce((total, file) => total + parseSizeMB(file.size), 0);
}

function formatCatalogSize(files: MalinaCatalogFile[]): string {
  const mb = bundleSizeMB(files);
  return mb >= 1000 ? `~${(mb / 1000).toFixed(1)} GB` : `~${mb.toFixed(0)} MB`;
}

function formatBytes(bytes: number): string {
  if (!bytes || bytes <= 0) return '';
  const mb = bytes / 1_000_000;
  return mb >= 1000 ? `${(mb / 1000).toFixed(2)} GB` : `${mb.toFixed(1)} MB`;
}

function sourceName(source?: string): string {
  if (!source) return '';
  try {
    return decodeURIComponent(new URL(source).pathname.split('/').pop() || source);
  } catch {
    return source;
  }
}

export default function MalinaModels() {
  const [catalog, setCatalog] = useState<MalinaCatalogEntry[]>([]);
  const [installed, setInstalled] = useState<MalinaModelEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [sortColumn, setSortColumn] = useState<SortColumn>('name');
  const [sortDir, setSortDir] = useState<SortDir>('asc');
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const { download, isDownloading, startMalinaDownload, cancelDownload } = useDownload();

  const loadInstalled = useCallback(async () => {
    try {
      const response = await api.listMalinaModels();
      setInstalled(response.models ?? []);
    } catch {
      setInstalled([]);
    }
  }, []);

  const loadCatalog = useCallback(async () => {
    try {
      const response = await api.listMalinaCatalog();
      setCatalog(response.models ?? []);
    } catch (err) {
      setError(`Failed to load catalog: ${(err as Error).message}`);
      setCatalog([]);
    }
  }, []);

  useEffect(() => {
    void loadCatalog().finally(() => setLoading(false));
    void loadInstalled();
  }, [loadCatalog, loadInstalled]);

  useEffect(() => {
    if (download?.origin === 'malina' && download.status === 'complete') {
      void loadInstalled();
    }
  }, [download?.origin, download?.status, loadInstalled]);

  const installedMap = useMemo(() => new Map(installed.map((entry) => [entry.id, entry])), [installed]);
  const sortedCatalog = useMemo(() => [...catalog].sort((a, b) => {
    const comparison = sortColumn === 'name'
      ? a.id.localeCompare(b.id)
      : bundleSizeMB(a.files) - bundleSizeMB(b.files);
    return sortDir === 'asc' ? comparison : -comparison;
  }), [catalog, sortColumn, sortDir]);

  const toggleSort = (column: SortColumn) => {
    if (sortColumn === column) {
      setSortDir((current) => current === 'asc' ? 'desc' : 'asc');
      return;
    }
    setSortColumn(column);
    setSortDir('asc');
  };

  const sortIndicator = (column: SortColumn) => sortColumn === column ? (sortDir === 'asc' ? ' ▲' : ' ▼') : '';

  const handlePull = (entry: MalinaCatalogEntry) => {
    startMalinaDownload(entry.id);
  };

  const handleRemove = async (id: string) => {
    if (!confirm(`Remove Malina model bundle "${id}"?`)) return;
    try {
      await api.removeMalinaModel(id);
      await loadInstalled();
    } catch (err) {
      setError(`Remove failed: ${(err as Error).message}`);
    }
  };

  const headerStyle = { textAlign: 'left' as const, padding: 8 };
  const sortHeaderStyle = { ...headerStyle, cursor: 'pointer' as const, userSelect: 'none' as const };

  return (
    <div>
      <div className="page-header">
        <h2>Stable Diffusion Models</h2>
        <p>Download and manage the curated Malina model bundles included with this Kronk version.</p>
      </div>

      {error && <div className="status-box" style={{ marginBottom: 16 }}><div className="status-line error">{error}</div></div>}

      <div className="card">
        {loading ? <p>Loading catalog…</p> : (
          <table className="catalog-table">
            <thead>
              <tr>
                <th style={sortHeaderStyle} onClick={() => toggleSort('name')}>Name{sortIndicator('name')}</th>
                <th style={sortHeaderStyle} onClick={() => toggleSort('size')}>Size{sortIndicator('size')}</th>
                <th style={headerStyle}>Capability</th>
                <th style={headerStyle}>Installed</th>
                <th style={headerStyle}>Status</th>
                <th style={{ ...headerStyle, textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {sortedCatalog.map((entry) => {
                const installedEntry = installedMap.get(entry.id);
                const state = download?.origin === 'malina' && download.modelUrl === entry.id ? download : null;
                const inFlight = state?.status === 'downloading';
                const current = state?.progress?.currentBytes ?? 0;
                const total = state?.progress?.totalBytes ?? 0;
                const percent = total > 0 ? Math.min(100, Math.round((current / total) * 100)) : 0;
                const selected = selectedID === entry.id;
                const lastMessage = state?.messages[state.messages.length - 1];

                return (
                  <Fragment key={entry.id}>
                    <tr className={selected ? 'active' : ''} onClick={() => setSelectedID(selected ? null : entry.id)}>
                      <td style={{ fontFamily: 'var(--font-mono, monospace)' }}>{entry.id}{entry.gated ? ' 🔒' : ''}</td>
                      <td>{formatCatalogSize(entry.files)}</td>
                      <td>{entry.basic_text_to_image ? 'Text / image generation' : 'Support bundle'}</td>
                      <td>{installedEntry ? '✓' : '—'}</td>
                      <td style={{ fontSize: 13 }}>
                        {state ? (
                          <div>
                            <div>{lastMessage?.text || state.status}{state.progress?.src ? ` ${sourceName(state.progress.src)}` : ''}{inFlight && total > 0 ? ` — ${formatBytes(current)} / ${formatBytes(total)} (${percent}%)${state.progress?.mbPerSec ? ` @ ${state.progress.mbPerSec.toFixed(1)} MB/s` : ''}` : ''}</div>
                            {inFlight && total > 0 && (
                              <div className="model-download-progress-track">
                                <div className="model-download-progress-fill" style={{ width: `${percent}%` }} />
                              </div>
                            )}
                            {lastMessage?.type === 'error' && <div className="status-line error" style={{ marginTop: 4 }}>{lastMessage.text}</div>}
                          </div>
                        ) : installedEntry ? `installed (${formatBytes(installedEntry.size)})` : '—'}
                      </td>
                      <td style={{ textAlign: 'right' }} onClick={(event) => event.stopPropagation()}>
                        {inFlight ? (
                          <button className="btn btn-danger" onClick={cancelDownload}>Cancel</button>
                        ) : installedEntry ? (
                          <button className="btn btn-danger" onClick={() => handleRemove(entry.id)}>Remove</button>
                        ) : (
                          <button className="btn btn-primary" onClick={() => handlePull(entry)} disabled={isDownloading}>Pull</button>
                        )}
                      </td>
                    </tr>
                    {selected && (
                      <tr>
                        <td colSpan={6} style={{ padding: '20px 24px', background: 'var(--color-active-item-bg)' }}>
                          <p style={{ margin: '0 0 10px' }}>{entry.description}</p>
                          <div style={{ marginBottom: 10 }}><strong>License:</strong> {entry.license}{entry.gated ? ' · Hugging Face access required' : ''}</div>
                          <table style={{ width: '100%', borderCollapse: 'collapse', fontSize: 13 }}>
                            <thead><tr><th style={headerStyle}>Role</th><th style={headerStyle}>File</th><th style={headerStyle}>Size</th></tr></thead>
                            <tbody>{entry.files.map((file) => (
                              <tr key={`${file.role}-${file.filename}`}>
                                <td style={{ padding: 8 }}>{file.role}</td>
                                <td style={{ padding: 8, fontFamily: 'var(--font-mono, monospace)' }}>{file.filename}</td>
                                <td style={{ padding: 8 }}>{file.size}</td>
                              </tr>
                            ))}</tbody>
                          </table>
                        </td>
                      </tr>
                    )}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
