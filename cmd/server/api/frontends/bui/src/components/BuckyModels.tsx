import { useState, useRef, useEffect, useCallback, useMemo, Fragment } from 'react';
import { api } from '../services/api';
import type {
  BuckyCatalogEntry,
  BuckyModelEntry,
  BuckyModelDetails,
  PullResponse,
} from '../types';

interface PullState {
  status: string;
  currentBytes?: number;
  totalBytes?: number;
  mbPerSec?: number;
  error?: string;
}

type SortColumn = 'name' | 'size';
type SortDir = 'asc' | 'desc';

// parseSizeMB converts a catalog size string like "75 MB" or "2.9 GB"
// into a numeric MB value for sortable comparison.
function parseSizeMB(s: string): number {
  const m = /^([\d.]+)\s*(MB|GB|KB|B)?$/i.exec(s.trim());
  if (!m) return 0;
  const n = parseFloat(m[1]);
  const unit = (m[2] || 'MB').toUpperCase();
  switch (unit) {
    case 'GB': return n * 1024;
    case 'KB': return n / 1024;
    case 'B':  return n / (1024 * 1024);
    default:   return n;
  }
}

export default function BuckyModels() {
  const [catalog, setCatalog] = useState<BuckyCatalogEntry[]>([]);
  const [installed, setInstalled] = useState<BuckyModelEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [pullStates, setPullStates] = useState<Record<string, PullState>>({});
  const [sortCol, setSortCol] = useState<SortColumn>('name');
  const [sortDir, setSortDir] = useState<SortDir>('asc');
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [detailsByID, setDetailsByID] = useState<Record<string, BuckyModelDetails>>({});
  const [detailsLoading, setDetailsLoading] = useState<Record<string, boolean>>({});
  const [detailsError, setDetailsError] = useState<Record<string, string>>({});
  const cancelRefs = useRef<Record<string, (() => void) | null>>({});

  const loadInstalled = useCallback(async () => {
    try {
      const resp = await api.listBuckyModels();
      setInstalled(resp.models ?? []);
    } catch {
      setInstalled([]);
    }
  }, []);

  const loadCatalog = useCallback(async () => {
    try {
      const resp = await api.listBuckyCatalog();
      setCatalog(resp.models ?? []);
    } catch (err) {
      setError(`Failed to load catalog: ${(err as Error).message}`);
      setCatalog([]);
    }
  }, []);

  useEffect(() => {
    Promise.all([loadCatalog(), loadInstalled()]).finally(() => setLoading(false));
  }, [loadCatalog, loadInstalled]);

  const installedMap = useMemo(() => {
    const m = new Map<string, BuckyModelEntry>();
    for (const e of installed) m.set(e.id, e);
    return m;
  }, [installed]);

  const sorted = useMemo(() => {
    const arr = [...catalog];
    arr.sort((a, b) => {
      let cmp = 0;
      if (sortCol === 'name') {
        cmp = a.id.localeCompare(b.id);
      } else {
        cmp = parseSizeMB(a.size) - parseSizeMB(b.size);
      }
      return sortDir === 'asc' ? cmp : -cmp;
    });
    return arr;
  }, [catalog, sortCol, sortDir]);

  const toggleSort = (col: SortColumn) => {
    if (sortCol === col) {
      setSortDir((d) => (d === 'asc' ? 'desc' : 'asc'));
    } else {
      setSortCol(col);
      setSortDir('asc');
    }
  };

  const sortIndicator = (col: SortColumn) => {
    if (sortCol !== col) return '';
    return sortDir === 'asc' ? ' ▲' : ' ▼';
  };

  const handlePull = (entry: BuckyCatalogEntry) => {
    setPullStates((prev) => ({ ...prev, [entry.id]: { status: 'starting' } }));

    cancelRefs.current[entry.id] = api.pullBuckyModel(
      entry.id,
      (data: PullResponse) => {
        setPullStates((prev) => ({
          ...prev,
          [entry.id]: {
            status: data.status,
            currentBytes: data.progress?.current_bytes,
            totalBytes: data.progress?.total_bytes,
            mbPerSec: data.progress?.mb_per_sec,
          },
        }));
      },
      (errMsg: string) => {
        setPullStates((prev) => ({
          ...prev,
          [entry.id]: { ...(prev[entry.id] || { status: 'error' }), status: 'error', error: errMsg },
        }));
        cancelRefs.current[entry.id] = null;
      },
      () => {
        setPullStates((prev) => ({
          ...prev,
          [entry.id]: { ...(prev[entry.id] || { status: 'complete' }), status: 'complete' },
        }));
        cancelRefs.current[entry.id] = null;
        loadInstalled();
      },
    );
  };

  const handleCancel = (id: string) => {
    const cancel = cancelRefs.current[id];
    if (cancel) {
      cancel();
      cancelRefs.current[id] = null;
    }
    setPullStates((prev) => ({
      ...prev,
      [id]: { ...(prev[id] || { status: 'cancelled' }), status: 'cancelled' },
    }));
  };

  const handleRemove = async (id: string) => {
    if (!confirm(`Remove whisper model "${id}"?`)) return;
    try {
      await api.removeBuckyModel(id);
      loadInstalled();
    } catch (err) {
      setError(`Remove failed: ${(err as Error).message}`);
    }
  };

  const handleSelect = (id: string) => {
    if (selectedID === id) {
      setSelectedID(null);
      return;
    }
    setSelectedID(id);

    if (detailsByID[id] || detailsLoading[id]) return;

    setDetailsLoading((prev) => ({ ...prev, [id]: true }));
    setDetailsError((prev) => { const { [id]: _, ...rest } = prev; return rest; });
    api.getBuckyModelDetails(id)
      .then((d) => setDetailsByID((prev) => ({ ...prev, [id]: d })))
      .catch((err) => setDetailsError((prev) => ({ ...prev, [id]: (err as Error).message })))
      .finally(() => setDetailsLoading((prev) => { const { [id]: _, ...rest } = prev; return rest; }));
  };

  const headerStyle = { textAlign: 'left' as const, padding: 8 };
  const sortHeaderStyle = { ...headerStyle, cursor: 'pointer' as const, userSelect: 'none' as const };

  return (
    <div>
      <div className="page-header">
        <h2>Whisper Models</h2>
        <p>Download and manage whisper.cpp models. Pulling a model fetches a ggml binary from the upstream HuggingFace mirror.</p>
      </div>

      {error && (
        <div className="status-box" style={{ marginBottom: 16 }}>
          <div className="status-line error">{error}</div>
        </div>
      )}

      <div className="card">
        {loading ? (
          <p>Loading catalog…</p>
        ) : (
          <table className="catalog-table">
            <thead>
              <tr>
                <th style={sortHeaderStyle} onClick={() => toggleSort('name')}>Name{sortIndicator('name')}</th>
                <th style={sortHeaderStyle} onClick={() => toggleSort('size')}>Size{sortIndicator('size')}</th>
                <th style={headerStyle}>Notes</th>
                <th style={headerStyle}>Installed</th>
                <th style={headerStyle}>Status</th>
                <th style={{ ...headerStyle, textAlign: 'right' }}>Actions</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((entry) => {
                const installedEntry = installedMap.get(entry.id);
                const isInstalled = !!installedEntry;
                const state = pullStates[entry.id];
                const inFlight = !!state && state.status !== 'complete' && state.status !== 'error' && state.status !== 'cancelled';
                const total = state?.totalBytes ?? 0;
                const cur = state?.currentBytes ?? 0;
                const pct = total > 0 ? Math.min(100, Math.round((cur / total) * 100)) : 0;
                const isSelected = selectedID === entry.id;
                const stop = (e: React.MouseEvent) => e.stopPropagation();

                return (
                  <Fragment key={entry.id}>
                    <tr
                      className={isSelected ? 'active' : ''}
                      onClick={() => handleSelect(entry.id)}
                    >
                      <td style={{ fontFamily: 'var(--font-mono, monospace)' }}>{entry.id}</td>
                      <td>{entry.size}</td>
                      <td style={{ color: isSelected ? 'inherit' : 'var(--color-gray-600)', fontSize: 13 }}>{entry.notes}</td>
                      <td>{isInstalled ? '✓' : '—'}</td>
                      <td style={{ fontSize: 13 }}>
                        {state ? (
                          <div>
                            <div>
                              {state.status}
                              {inFlight && total > 0 && (
                                <> — {formatBytes(cur)} / {formatBytes(total)} ({pct}%) {state.mbPerSec ? `@ ${state.mbPerSec.toFixed(1)} MB/s` : ''}</>
                              )}
                            </div>
                            {inFlight && total > 0 && (
                              <div style={{ marginTop: 4, height: 6, background: 'var(--color-gray-200)', borderRadius: 3, overflow: 'hidden' }}>
                                <div style={{ width: `${pct}%`, height: '100%', background: 'var(--color-primary, #2563eb)', transition: 'width 200ms linear' }} />
                              </div>
                            )}
                            {state.error && (
                              <div className="status-line error" style={{ marginTop: 4 }}>{state.error}</div>
                            )}
                          </div>
                        ) : isInstalled ? (
                          <span style={{ color: isSelected ? 'inherit' : 'var(--color-gray-600)' }}>
                            installed{installedEntry.size > 0 ? ` (${formatBytes(installedEntry.size)})` : ''}
                          </span>
                        ) : (
                          <span style={{ color: isSelected ? 'inherit' : 'var(--color-gray-600)' }}>—</span>
                        )}
                      </td>
                      <td style={{ textAlign: 'right' }} onClick={stop}>
                        {inFlight ? (
                          <button className="btn btn-danger" onClick={() => handleCancel(entry.id)}>Cancel</button>
                        ) : isInstalled ? (
                          <button className="btn btn-danger" onClick={() => handleRemove(entry.id)}>Remove</button>
                        ) : (
                          <button className="btn btn-primary" onClick={() => handlePull(entry)}>Pull</button>
                        )}
                      </td>
                    </tr>
                    {isSelected && (
                      <tr>
                        <td colSpan={6} style={{ padding: '20px 24px', background: 'var(--color-active-item-bg)' }}>
                          <DetailsPanel
                            details={detailsByID[entry.id]}
                            loading={!!detailsLoading[entry.id]}
                            error={detailsError[entry.id]}
                          />
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

function formatBytes(n: number): string {
  if (!n || n <= 0) return '';
  const mb = n / (1000 * 1000);
  if (mb < 1024) return `${mb.toFixed(1)} MB`;
  return `${(mb / 1024).toFixed(2)} GB`;
}

interface DetailsPanelProps {
  details?: BuckyModelDetails;
  loading: boolean;
  error?: string;
}

function DetailsPanel({ details, loading, error }: DetailsPanelProps) {
  if (loading) return <span style={{ color: 'var(--color-gray-600)' }}>Loading details…</span>;
  if (error) return <span className="status-line error">Details error: {error}</span>;
  if (!details) return null;

  const rows: Array<[string, React.ReactNode]> = [
    ['Model Type', details.model_type],
    ['Multilingual', details.is_multilingual ? 'yes' : 'no (english-only)'],
    ['Quantization', details.quantization],
    ['Quant Version', details.qnt_version],
    ['n_vocab', details.n_vocab],
    ['n_audio_ctx', details.n_audio_ctx],
    ['n_audio_state', details.n_audio_state],
    ['n_audio_head', details.n_audio_head],
    ['n_audio_layer', details.n_audio_layer],
    ['n_text_ctx', details.n_text_ctx],
    ['n_text_state', details.n_text_state],
    ['n_text_head', details.n_text_head],
    ['n_text_layer', details.n_text_layer],
    ['n_mels', details.n_mels],
  ];

  return (
    <div style={{ display: 'flex', flexWrap: 'wrap', gap: '6px 20px', fontSize: 15 }}>
      {rows.map(([label, value]) => (
        <div key={label} style={{ display: 'flex', alignItems: 'baseline', gap: 6 }}>
          <span style={{ color: 'var(--color-gray-600)' }}>{label}:</span>
          <span style={{ fontFamily: 'var(--font-mono, monospace)' }}>{value}</span>
        </div>
      ))}
    </div>
  );
}
