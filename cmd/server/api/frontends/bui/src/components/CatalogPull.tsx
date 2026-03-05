import { useState, useEffect } from 'react';
import { api } from '../services/api';
import { useDownload } from '../contexts/DownloadContext';
import type { CatalogModelsResponse } from '../types';
import DownloadInfoTable from './DownloadInfoTable';
import DownloadProgressBar from './DownloadProgressBar';

export default function CatalogPull() {
  const { download, isDownloading, startCatalogDownload, cancelDownload } = useDownload();
  const [catalogList, setCatalogList] = useState<CatalogModelsResponse | null>(null);
  const [selectedId, setSelectedId] = useState('');
  const [listLoading, setListLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadCatalogList();
  }, []);

  useEffect(() => {
    if (download?.kind === 'catalog' && download.status === 'complete') {
      loadCatalogList();
    }
  }, [download?.status]);

  const loadCatalogList = async () => {
    setListLoading(true);
    try {
      const response = await api.listCatalog();
      setCatalogList(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load catalog');
    } finally {
      setListLoading(false);
    }
  };

  const handlePull = () => {
    if (!selectedId) return;
    setError(null);
    startCatalogDownload(selectedId);
  };

  const isCatalogDownload = download?.kind === 'catalog' && download.catalogId === selectedId;
  const pulling = isCatalogDownload ? download.status === 'downloading' : false;
  const pullMessages = isCatalogDownload ? download.messages : [];

  return (
    <div>
      <div className="page-header">
        <h2>Pull Catalog Model</h2>
        <p>Download a model from the catalog</p>
      </div>

      <div className="card">
        {error && <div className="alert alert-error">{error}</div>}

        {listLoading ? (
          <div className="loading">Loading catalog</div>
        ) : (
          <>
            <div className="form-group">
              <label htmlFor="modelSelect">Select Model</label>
              <select
                id="modelSelect"
                value={selectedId}
                onChange={(e) => setSelectedId(e.target.value)}
                disabled={isDownloading}
              >
                <option value="">-- Select a model --</option>
                {catalogList?.map((model) => (
                  <option key={model.id} value={model.id}>
                    {model.id} {model.downloaded ? '(downloaded)' : ''}
                  </option>
                ))}
              </select>
            </div>

            <div style={{ display: 'flex', gap: '12px' }}>
              <button
                className="btn btn-primary"
                onClick={handlePull}
                disabled={!selectedId || isDownloading}
              >
                {pulling ? 'Pulling...' : 'Pull Model'}
              </button>
              {pulling && (
                <button className="btn btn-danger" onClick={cancelDownload}>
                  Cancel
                </button>
              )}
            </div>
          </>
        )}

        {isCatalogDownload && download.meta && (
          <DownloadInfoTable meta={download.meta} />
        )}

        {isCatalogDownload && download.progress && pulling && (
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
    </div>
  );
}
