import { useEffect, useState } from 'react';
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
  const [pullingId, setPullingId] = useState<string | null>(null);

  const [sortField, setSortField] = useState<SortField>('id');
  const [sortAsc, setSortAsc] = useState(true);

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

  // Refresh local models after a successful pull so rows flip to "downloaded".
  useEffect(() => {
    if (download?.kind === 'catalog' && download.status === 'complete' && pullingId) {
      invalidateLocal();
      loadLocalModels();
      setPullingId(null);
    }
    if (download?.status === 'error' && pullingId) {
      setPullingId(null);
    }
  }, [download?.status, download?.kind, pullingId, invalidateLocal, loadLocalModels]);

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

  const handlePull = (id: string) => {
    if (isDownloading) return;
    setPullingId(id);
    startCatalogDownload(id, host.trim() || undefined);
  };

  const isCatalogDownload = download?.kind === 'catalog' && download.catalogId === pullingId;
  const pulling = isCatalogDownload ? download.status === 'downloading' : false;
  const pullMessages = isCatalogDownload ? download.messages : [];

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
              disabled={loading}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !loading) {
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
            disabled={loading || !host.trim()}
          >
            {loading ? 'Connecting…' : 'Connect'}
          </button>
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
                    <th style={{ width: '160px' }}></th>
                  </tr>
                </thead>
                <tbody>
                  {sortedModels.map((model) => {
                    const haveLocally = localIds.has(model.id);
                    const isThisRowPulling = pullingId === model.id && pulling;
                    return (
                      <tr key={model.id}>
                        <td style={{ textAlign: 'center', color: model.validated ? 'inherit' : 'var(--color-error)' }}>
                          {model.validated ? '✓' : '✗'}
                        </td>
                        <td><span className="catalog-table-cell-ellipsis">{model.id}</span></td>
                        <td>{model.owned_by || '-'}</td>
                        <td>{model.model_family || '-'}</td>
                        <td style={{ textAlign: 'center' }}>{model.has_projection ? '✓' : ''}</td>
                        <td>{formatBytes(model.size)}</td>
                        <td style={{ textAlign: 'right' }}>
                          {haveLocally ? (
                            <span className="badge badge-yes">Downloaded</span>
                          ) : (
                            <button
                              type="button"
                              className="btn btn-primary"
                              onClick={() => handlePull(model.id)}
                              disabled={isDownloading}
                              style={{ padding: '4px 10px', fontSize: '13px' }}
                            >
                              {isThisRowPulling ? 'Pulling…' : 'Pull'}
                            </button>
                          )}
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

          <div style={{ display: 'flex', gap: '12px', marginBottom: '12px' }}>
            {pulling && (
              <button className="btn btn-danger" type="button" onClick={cancelDownload}>
                Cancel
              </button>
            )}
            {!pulling && (
              <button className="btn" type="button" onClick={clearDownload}>
                Clear progress
              </button>
            )}
          </div>

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
