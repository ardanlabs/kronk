import { useState, useEffect } from 'react';
import { api } from '../services/api';
import type { CatalogModelResponse, CatalogModelsResponse } from '../types';

export default function CatalogShow() {
  const [catalogList, setCatalogList] = useState<CatalogModelsResponse | null>(null);
  const [selectedId, setSelectedId] = useState('');
  const [data, setData] = useState<CatalogModelResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [listLoading, setListLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadCatalogList();
  }, []);

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

  const handleSelect = async (id: string) => {
    setSelectedId(id);
    if (!id) {
      setData(null);
      return;
    }

    setLoading(true);
    setError(null);
    try {
      const response = await api.showCatalogModel(id);
      setData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load model info');
      setData(null);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Show Catalog Model</h2>
        <p>View detailed information about a catalog model</p>
      </div>

      <div className="card">
        {listLoading ? (
          <div className="loading">Loading catalog</div>
        ) : (
          <div className="form-group">
            <label htmlFor="modelSelect">Select Model</label>
            <select
              id="modelSelect"
              value={selectedId}
              onChange={(e) => handleSelect(e.target.value)}
              disabled={loading}
            >
              <option value="">-- Select a model --</option>
              {catalogList?.map((model) => (
                <option key={model.id} value={model.id}>
                  {model.id}
                </option>
              ))}
            </select>
          </div>
        )}
      </div>

      {loading && <div className="loading">Loading model details</div>}

      {error && <div className="alert alert-error">{error}</div>}

      {data && (
        <div className="card">
          <h3 style={{ marginBottom: '16px' }}>{data.id}</h3>

          <div className="model-meta">
            <div className="model-meta-item">
              <label>Category</label>
              <span>{data.category}</span>
            </div>
            <div className="model-meta-item">
              <label>Owner</label>
              <span>{data.owned_by}</span>
            </div>
            <div className="model-meta-item">
              <label>Family</label>
              <span>{data.model_family}</span>
            </div>
            <div className="model-meta-item">
              <label>Downloaded</label>
              <span className={`badge ${data.downloaded ? 'badge-yes' : 'badge-no'}`}>
                {data.downloaded ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Endpoint</label>
              <span>{data.capabilities.endpoint}</span>
            </div>
            <div className="model-meta-item">
              <label>Web Page</label>
              <span>
                {data.web_page ? (
                  <a href={data.web_page} target="_blank" rel="noopener noreferrer">
                    {data.web_page}
                  </a>
                ) : (
                  '-'
                )}
              </span>
            </div>
          </div>

          <div style={{ marginTop: '24px' }}>
            <h4 style={{ marginBottom: '12px' }}>Capabilities</h4>
            <div className="model-meta">
              <div className="model-meta-item">
                <label>Images</label>
                <span className={`badge ${data.capabilities.images ? 'badge-yes' : 'badge-no'}`}>
                  {data.capabilities.images ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Audio</label>
                <span className={`badge ${data.capabilities.audio ? 'badge-yes' : 'badge-no'}`}>
                  {data.capabilities.audio ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Video</label>
                <span className={`badge ${data.capabilities.video ? 'badge-yes' : 'badge-no'}`}>
                  {data.capabilities.video ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Streaming</label>
                <span className={`badge ${data.capabilities.streaming ? 'badge-yes' : 'badge-no'}`}>
                  {data.capabilities.streaming ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Reasoning</label>
                <span className={`badge ${data.capabilities.reasoning ? 'badge-yes' : 'badge-no'}`}>
                  {data.capabilities.reasoning ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Tooling</label>
                <span className={`badge ${data.capabilities.tooling ? 'badge-yes' : 'badge-no'}`}>
                  {data.capabilities.tooling ? 'Yes' : 'No'}
                </span>
              </div>
            </div>
          </div>

          <div style={{ marginTop: '24px' }}>
            <h4 style={{ marginBottom: '12px' }}>Files</h4>
            <div className="model-meta">
              <div className="model-meta-item">
                <label>Model</label>
                <span>
                  {data.files.model.url || '-'} {data.files.model.size && `(${data.files.model.size})`}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Projection</label>
                <span>
                  {data.files.proj.url || '-'} {data.files.proj.size && `(${data.files.proj.size})`}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Jinja</label>
                <span>
                  {data.files.jinja.url || '-'} {data.files.jinja.size && `(${data.files.jinja.size})`}
                </span>
              </div>
            </div>
          </div>

          {data.metadata.description && (
            <div style={{ marginTop: '24px' }}>
              <h4 style={{ marginBottom: '12px' }}>Description</h4>
              <p>{data.metadata.description}</p>
            </div>
          )}

          <div style={{ marginTop: '24px' }}>
            <h4 style={{ marginBottom: '12px' }}>Metadata</h4>
            <div className="model-meta">
              <div className="model-meta-item">
                <label>Created</label>
                <span>{new Date(data.metadata.created).toLocaleString()}</span>
              </div>
              <div className="model-meta-item">
                <label>Collections</label>
                <span>{data.metadata.collections || '-'}</span>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
