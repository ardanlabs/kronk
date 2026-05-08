import { useEffect, useRef, useState } from 'react';
import { api } from '../services/api';
import { useDownload } from '../contexts/DownloadContext';
import { useModelList } from '../contexts/ModelListContext';
import type { PeerModelDetail } from '../types';
import { formatBytes } from '../lib/format';
import { FieldLabel } from './ParamTooltips';
import DownloadInfoTable from './DownloadInfoTable';
import DownloadProgressBar from './DownloadProgressBar';

const PEER_HOST_STORAGE_KEY = 'kronk_kms_peer_server';

type SortField = 'id' | 'owned_by' | 'model_family' | 'size' | 'has_projection' | 'validated';

type RowStatus = 'idle' | 'queued' | 'pulling' | 'done' | 'error';

function getSortValue(model: PeerModelDetail, field: SortField): string | number {
  switch (field) {
    case 'id': return model.id.toLowerCase();
    case 'owned_by': return (model.owned_by || '').toLowerCase();
    case 'model_family': return (model.model_family || '').toLowerCase();
    case 'has_projection': return model.has_projection ? 1 : 0;
    case 'validated': return model.validated ? 1 : 0;
    case 'size': return model.size;
    default: return '';
  }
}

export default function KMSPull() {
  const { download, isDownloading, startCatalogDownload, cancelDownload, clearDownload } = useDownload();
  const { models: localModels, loadModels: loadLocalModels, invalidate: invalidateLocal } = useModelList();

  const [host, setHost] = useState<string>(() => localStorage.getItem(PEER_HOST_STORAGE_KEY) || '');
  const [peerModels, setPeerModels] = useState<PeerModelDetail[] | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [sortField, setSortField] = useState<SortField>('id');
  const [sortAsc, setSortAsc] = useState(true);

  // Multi-select + batch queue state.
  const [checkedIds, setCheckedIds] = useState<Set<string>>(new Set());
  const [queue, setQueue] = useState<string[]>([]);
  const [queueIndex, setQueueIndex] = useState(0);
  const [rowStatus, setRowStatus] = useState<Record<string, RowStatus>>({});
  const [pullingId, setPullingId] = useState<string | null>(null);
  const cancelledRef = useRef(false);

  const queueRunning = queue.length > 0 && queueIndex < queue.length;

  const handleHostChange = (value: string) => {
    setHost(value);
    if (value) {
      localStorage.setItem(PEER_HOST_STORAGE_KEY, value);
    } else {
      localStorage.removeItem(PEER_HOST_STORAGE_KEY);
    }
  };

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortAsc(!sortAsc);
    } else {
      setSortField(field);
      setSortAsc(true);
    }
  };

  const handleConnect = async () => {
    const trimmed = host.trim();
    if (!trimmed) {
      setError('Peer server is required');
      return;
    }
    setLoading(true);
    setError(null);
    setPeerModels(null);
    setCheckedIds(new Set());
    setRowStatus({});
    setQueue([]);
    setQueueIndex(0);
    setPullingId(null);
    try {
      const [peerResp] = await Promise.all([
        api.listPeerModels(trimmed),
        loadLocalModels(),
      ]);
      setPeerModels(peerResp.models ?? []);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  };

  const localIds = new Set((localModels?.data ?? []).map((m) => m.id));

  const sortedModels = peerModels
    ? [...peerModels].sort((a, b) => {
        const va = getSortValue(a, sortField);
        const vb = getSortValue(b, sortField);
        const dir = sortAsc ? 1 : -1;
        if (typeof va === 'number' && typeof vb === 'number') {
          return (va - vb) * dir;
        }
        return String(va).localeCompare(String(vb)) * dir;
      })
    : [];

  const eligibleIds = sortedModels.filter((m) => !localIds.has(m.id)).map((m) => m.id);
  const allEligibleChecked =
    eligibleIds.length > 0 && eligibleIds.every((id) => checkedIds.has(id));

  const toggleChecked = (id: string) => {
    if (queueRunning || localIds.has(id)) return;
    setCheckedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id); else next.add(id);
      return next;
    });
  };

  const toggleAllEligible = () => {
    if (queueRunning) return;
    if (allEligibleChecked) {
      setCheckedIds(new Set());
    } else {
      setCheckedIds(new Set(eligibleIds));
    }
  };

  const handlePullOne = (id: string) => {
    if (isDownloading || queueRunning) return;
    setPullingId(id);
    setRowStatus({ [id]: 'pulling' });
    cancelledRef.current = false;
    setQueue([id]);
    setQueueIndex(0);
    startCatalogDownload(id, host.trim() || undefined);
  };

  const handlePullSelected = () => {
    if (isDownloading || queueRunning) return;
    const ids = sortedModels.filter((m) => checkedIds.has(m.id)).map((m) => m.id);
    if (ids.length === 0) return;

    cancelledRef.current = false;

    const initialStatus: Record<string, RowStatus> = {};
    ids.forEach((id) => { initialStatus[id] = 'queued'; });
    initialStatus[ids[0]] = 'pulling';
    setRowStatus(initialStatus);

    setQueue(ids);
    setQueueIndex(0);
    setPullingId(ids[0]);
    startCatalogDownload(ids[0], host.trim() || undefined);
  };

  const handleCancel = () => {
    cancelledRef.current = true;
    cancelDownload();
  };

  // React to download status changes to drive the queue forward.
  useEffect(() => {
    if (!queueRunning || !download) return;
    if (download.kind !== 'catalog') return;
    if (download.catalogId !== queue[queueIndex]) return;

    if (download.status === 'complete') {
      const finishedId = queue[queueIndex];
      setRowStatus((prev) => ({ ...prev, [finishedId]: 'done' }));
      invalidateLocal();
      void loadLocalModels();

      const next = queueIndex + 1;
      if (cancelledRef.current || next >= queue.length) {
        // Mark any not-yet-started rows as cancelled (back to idle).
        if (cancelledRef.current) {
          setRowStatus((prev) => {
            const out = { ...prev };
            for (let i = next; i < queue.length; i++) {
              if (out[queue[i]] === 'queued') delete out[queue[i]];
            }
            return out;
          });
        }
        setQueueIndex(queue.length);
        setPullingId(null);
        clearDownload();
        return;
      }

      const nextId = queue[next];
      setQueueIndex(next);
      setPullingId(nextId);
      setRowStatus((prev) => ({ ...prev, [nextId]: 'pulling' }));
      // Clear the prior download state and start the next pull.
      clearDownload();
      // Defer to next tick so the cleared state lands before the new start.
      setTimeout(() => startCatalogDownload(nextId, host.trim() || undefined), 0);
    }

    if (download.status === 'error') {
      const failedId = queue[queueIndex];
      setRowStatus((prev) => ({ ...prev, [failedId]: 'error' }));

      const next = queueIndex + 1;
      if (cancelledRef.current || next >= queue.length) {
        if (cancelledRef.current) {
          setRowStatus((prev) => {
            const out = { ...prev };
            for (let i = next; i < queue.length; i++) {
              if (out[queue[i]] === 'queued') delete out[queue[i]];
            }
            return out;
          });
        }
        setQueueIndex(queue.length);
        setPullingId(null);
        return;
      }

      const nextId = queue[next];
      setQueueIndex(next);
      setPullingId(nextId);
      setRowStatus((prev) => ({ ...prev, [nextId]: 'pulling' }));
      clearDownload();
      setTimeout(() => startCatalogDownload(nextId, host.trim() || undefined), 0);
    }
  }, [download?.status, download?.catalogId, download?.kind, queue, queueIndex, queueRunning, host, startCatalogDownload, clearDownload, invalidateLocal, loadLocalModels]);

  const isCatalogDownload = download?.kind === 'catalog' && pullingId !== null && download.catalogId === pullingId;
  const pulling = isCatalogDownload && download?.status === 'downloading';
  const pullMessages = isCatalogDownload ? (download?.messages ?? []) : [];

  const remaining = queue.length - queueIndex;

  const renderRowStatusCell = (id: string, haveLocally: boolean) => {
    const status = rowStatus[id];
    if (queueRunning) {
      if (status === 'pulling') return <span className="badge" style={{ background: 'var(--color-info, #2563eb)', color: 'white' }}>Pulling…</span>;
      if (status === 'queued') return <span className="badge badge-no">Queued</span>;
      if (status === 'done') return <span className="badge badge-yes">Downloaded</span>;
      if (status === 'error') return <span className="badge" style={{ background: 'var(--color-error, #dc2626)', color: 'white' }}>Failed</span>;
      if (haveLocally) return <span className="badge badge-yes">Downloaded</span>;
      return <span style={{ opacity: 0.5, fontSize: '12px' }}>—</span>;
    }
    // Idle (no queue running). Show post-queue results, otherwise the Pull button.
    if (status === 'done') return <span className="badge badge-yes">Downloaded</span>;
    if (status === 'error') return (
      <button
        type="button"
        className="btn btn-primary"
        onClick={() => handlePullOne(id)}
        disabled={isDownloading}
        style={{ padding: '4px 10px', fontSize: '13px' }}
        title="Retry pull"
      >
        Retry
      </button>
    );
    if (haveLocally) return <span className="badge badge-yes">Downloaded</span>;
    return (
      <button
        type="button"
        className="btn btn-primary"
        onClick={() => handlePullOne(id)}
        disabled={isDownloading}
        style={{ padding: '4px 10px', fontSize: '13px' }}
      >
        Pull
      </button>
    );
  };

  return (
    <div>
      <div className="page-header">
        <h2>KMS Pull Model</h2>
        <p className="page-description">
          Connect to another Kronk server on your local network to browse the models it has
          downloaded, and pull any of those models into this server. The peer must be running
          with the download endpoint enabled.
        </p>
      </div>

      <div className="card">
        <div style={{ display: 'flex', gap: '12px', alignItems: 'flex-end', flexWrap: 'wrap' }}>
          <div className="form-group" style={{ marginBottom: 0, flex: '0 0 280px' }}>
            <FieldLabel tooltipKey="peerKMSHost" htmlFor="kms-peer-host">
              Peer Server
            </FieldLabel>
            <input
              id="kms-peer-host"
              type="text"
              value={host}
              onChange={(e) => handleHostChange(e.target.value)}
              placeholder="192.168.0.246:11435"
              disabled={loading || queueRunning}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !loading && !queueRunning) {
                  e.preventDefault();
                  handleConnect();
                }
              }}
            />
          </div>
          <button
            className="btn btn-primary"
            type="button"
            onClick={handleConnect}
            disabled={loading || queueRunning || !host.trim()}
          >
            {loading ? 'Connecting…' : 'Connect'}
          </button>
          {peerModels && (
            <>
              <button
                className="btn btn-primary"
                type="button"
                onClick={handlePullSelected}
                disabled={queueRunning || isDownloading || checkedIds.size === 0}
                title={checkedIds.size === 0 ? 'Select one or more models to pull' : ''}
              >
                {queueRunning ? `Pulling ${queueIndex + 1} of ${queue.length}…` : `Pull Selected (${checkedIds.size})`}
              </button>
              {queueRunning && (
                <button className="btn btn-danger" type="button" onClick={handleCancel}>
                  Cancel {remaining > 1 ? `(${remaining} remaining)` : ''}
                </button>
              )}
            </>
          )}
        </div>

        {error && <div className="alert alert-error" style={{ marginTop: '12px' }}>{error}</div>}
      </div>

      {peerModels && (
        <div className="card" style={{ marginTop: '16px' }}>
          <div style={{ marginBottom: '12px', fontSize: '13px', color: 'var(--color-gray-600)' }}>
            {peerModels.length} model{peerModels.length === 1 ? '' : 's'} available on{' '}
            <code>{host.trim()}</code>
          </div>

          {peerModels.length === 0 ? (
            <div className="empty-state">
              <h3>No models on peer</h3>
              <p>The peer reported no installed models.</p>
            </div>
          ) : (
            <div className="catalog-table-wrap">
              <table className="catalog-table">
                <thead>
                  <tr>
                    <th style={{ width: '32px', textAlign: 'center' }}>
                      <input
                        type="checkbox"
                        aria-label="Select all eligible models"
                        checked={allEligibleChecked}
                        disabled={queueRunning || eligibleIds.length === 0}
                        onChange={toggleAllEligible}
                      />
                    </th>
                    <th
                      style={{ width: '40px', textAlign: 'center' }}
                      onClick={() => handleSort('validated')}
                      className="catalog-table-sortable"
                      title="Configuration and template confirmed working with the Kronk catalog"
                    >
                      VAL
                      <span className="catalog-table-sort-indicator">
                        {sortField === 'validated' ? (sortAsc ? ' ▲' : ' ▼') : ''}
                      </span>
                    </th>
                    {([
                      ['id', 'Model ID'],
                      ['owned_by', 'Provider'],
                      ['model_family', 'Family'],
                    ] as const).map(([field, label]) => (
                      <th
                        key={field}
                        onClick={() => handleSort(field)}
                        className="catalog-table-sortable"
                      >
                        {label}
                        <span className="catalog-table-sort-indicator">
                          {sortField === field ? (sortAsc ? ' ▲' : ' ▼') : ''}
                        </span>
                      </th>
                    ))}
                    <th
                      style={{ textAlign: 'center' }}
                      onClick={() => handleSort('has_projection')}
                      className="catalog-table-sortable"
                      title="Multimodal projection file present"
                    >
                      MTMD
                      <span className="catalog-table-sort-indicator">
                        {sortField === 'has_projection' ? (sortAsc ? ' ▲' : ' ▼') : ''}
                      </span>
                    </th>
                    <th onClick={() => handleSort('size')} className="catalog-table-sortable">
                      Size
                      <span className="catalog-table-sort-indicator">
                        {sortField === 'size' ? (sortAsc ? ' ▲' : ' ▼') : ''}
                      </span>
                    </th>
                    <th style={{ width: '160px', textAlign: 'right' }}>Status</th>
                  </tr>
                </thead>
                <tbody>
                  {sortedModels.map((model) => {
                    const haveLocally = localIds.has(model.id);
                    const checked = checkedIds.has(model.id);
                    const isCurrent = pullingId === model.id;
                    return (
                      <tr
                        key={model.id}
                        className={isCurrent ? 'active' : (checked ? 'active' : '')}
                        onClick={() => toggleChecked(model.id)}
                        style={{ cursor: queueRunning || haveLocally ? 'default' : 'pointer' }}
                      >
                        <td style={{ textAlign: 'center' }} onClick={(e) => e.stopPropagation()}>
                          <input
                            type="checkbox"
                            checked={checked}
                            disabled={queueRunning || haveLocally}
                            onChange={() => toggleChecked(model.id)}
                            aria-label={`Select ${model.id}`}
                          />
                        </td>
                        <td style={{ textAlign: 'center', color: model.validated ? 'inherit' : 'var(--color-error)' }}>
                          {model.validated ? '✓' : '✗'}
                        </td>
                        <td><span className="catalog-table-cell-ellipsis">{model.id}</span></td>
                        <td>{model.owned_by || '-'}</td>
                        <td>{model.model_family || '-'}</td>
                        <td style={{ textAlign: 'center' }}>{model.has_projection ? '✓' : ''}</td>
                        <td>{formatBytes(model.size)}</td>
                        <td style={{ textAlign: 'right' }} onClick={(e) => e.stopPropagation()}>
                          {renderRowStatusCell(model.id, haveLocally)}
                        </td>
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            </div>
          )}
        </div>
      )}

      {download && download.kind === 'catalog' && (download.status === 'downloading' || download.messages.length > 0) && (
        <div className="card" style={{ marginTop: '16px' }}>
          <h3 style={{ marginTop: 0, marginBottom: '12px' }}>Pull: {download.catalogId}</h3>

          {download.meta && <DownloadInfoTable meta={download.meta} />}
          {download.progress && pulling && (
            <DownloadProgressBar progress={download.progress} meta={download.meta} />
          )}
          {pullMessages.length > 0 && (
            <div className="status-box">
              {pullMessages.map((msg, idx) => (
                <div key={idx} className={`status-line ${msg.type}`}>
                  {msg.text}
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
