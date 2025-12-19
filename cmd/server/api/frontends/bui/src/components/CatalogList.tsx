import { useState, useEffect } from 'react';
import { api } from '../services/api';
import type { CatalogModelResponse, CatalogModelsResponse } from '../types';

export default function CatalogList() {
  const [data, setData] = useState<CatalogModelsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [modelInfo, setModelInfo] = useState<CatalogModelResponse | null>(null);
  const [infoLoading, setInfoLoading] = useState(false);
  const [infoError, setInfoError] = useState<string | null>(null);

  useEffect(() => {
    loadCatalog();
  }, []);

  const loadCatalog = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await api.listCatalog();
      setData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load catalog');
    } finally {
      setLoading(false);
    }
  };

  const handleRowClick = async (id: string) => {
    if (selectedId === id) {
      setSelectedId(null);
      setModelInfo(null);
      return;
    }

    setSelectedId(id);
    setInfoLoading(true);
    setInfoError(null);
    setModelInfo(null);

    try {
      const response = await api.showCatalogModel(id);
      setModelInfo(response);
    } catch (err) {
      setInfoError(err instanceof Error ? err.message : 'Failed to load model info');
    } finally {
      setInfoLoading(false);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Catalog</h2>
        <p>Browse available models in the catalog. Click a model to view details.</p>
      </div>

      <div className="card">
        {loading && <div className="loading">Loading catalog</div>}

        {error && <div className="alert alert-error">{error}</div>}

        {!loading && !error && data && (
          <div className="table-container">
            {data.length > 0 ? (
              <table>
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Category</th>
                    <th>Owner</th>
                    <th>Family</th>
                    <th>Downloaded</th>
                    <th>Capabilities</th>
                  </tr>
                </thead>
                <tbody>
                  {data.map((model) => (
                    <tr
                      key={model.id}
                      onClick={() => handleRowClick(model.id)}
                      className={selectedId === model.id ? 'selected' : ''}
                      style={{ cursor: 'pointer' }}
                    >
                      <td>{model.id}</td>
                      <td>{model.category}</td>
                      <td>{model.owned_by}</td>
                      <td>{model.model_family}</td>
                      <td>
                        <span className={`badge ${model.downloaded ? 'badge-yes' : 'badge-no'}`}>
                          {model.downloaded ? 'Yes' : 'No'}
                        </span>
                      </td>
                      <td>
                        {model.capabilities.images && (
                          <span className="badge badge-yes" style={{ marginRight: 4 }}>
                            Images
                          </span>
                        )}
                        {model.capabilities.audio && (
                          <span className="badge badge-yes" style={{ marginRight: 4 }}>
                            Audio
                          </span>
                        )}
                        {model.capabilities.video && (
                          <span className="badge badge-yes" style={{ marginRight: 4 }}>
                            Video
                          </span>
                        )}
                        {model.capabilities.streaming && (
                          <span className="badge badge-yes" style={{ marginRight: 4 }}>
                            Streaming
                          </span>
                        )}
                        {model.capabilities.reasoning && (
                          <span className="badge badge-yes" style={{ marginRight: 4 }}>
                            Reasoning
                          </span>
                        )}
                        {model.capabilities.tooling && (
                          <span className="badge badge-yes" style={{ marginRight: 4 }}>
                            Tooling
                          </span>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div className="empty-state">
                <h3>No catalog entries</h3>
                <p>The catalog is empty</p>
              </div>
            )}
          </div>
        )}

        <div style={{ marginTop: '16px' }}>
          <button
            className="btn btn-secondary"
            onClick={() => {
              loadCatalog();
              setSelectedId(null);
              setModelInfo(null);
            }}
            disabled={loading}
          >
            Refresh
          </button>
        </div>
      </div>

      {infoError && <div className="alert alert-error">{infoError}</div>}

      {infoLoading && (
        <div className="card">
          <div className="loading">Loading model details</div>
        </div>
      )}

      {modelInfo && !infoLoading && (
        <div className="card">
          <h3 style={{ marginBottom: '16px' }}>{modelInfo.id}</h3>

          <div className="model-meta">
            <div className="model-meta-item">
              <label>Category</label>
              <span>{modelInfo.category}</span>
            </div>
            <div className="model-meta-item">
              <label>Owner</label>
              <span>{modelInfo.owned_by}</span>
            </div>
            <div className="model-meta-item">
              <label>Family</label>
              <span>{modelInfo.model_family}</span>
            </div>
            <div className="model-meta-item">
              <label>Downloaded</label>
              <span className={`badge ${modelInfo.downloaded ? 'badge-yes' : 'badge-no'}`}>
                {modelInfo.downloaded ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Endpoint</label>
              <span>{modelInfo.capabilities.endpoint}</span>
            </div>
            <div className="model-meta-item">
              <label>Web Page</label>
              <span>
                {modelInfo.web_page ? (
                  <a href={modelInfo.web_page} target="_blank" rel="noopener noreferrer">
                    {modelInfo.web_page}
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
                <span className={`badge ${modelInfo.capabilities.images ? 'badge-yes' : 'badge-no'}`}>
                  {modelInfo.capabilities.images ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Audio</label>
                <span className={`badge ${modelInfo.capabilities.audio ? 'badge-yes' : 'badge-no'}`}>
                  {modelInfo.capabilities.audio ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Video</label>
                <span className={`badge ${modelInfo.capabilities.video ? 'badge-yes' : 'badge-no'}`}>
                  {modelInfo.capabilities.video ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Streaming</label>
                <span className={`badge ${modelInfo.capabilities.streaming ? 'badge-yes' : 'badge-no'}`}>
                  {modelInfo.capabilities.streaming ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Reasoning</label>
                <span className={`badge ${modelInfo.capabilities.reasoning ? 'badge-yes' : 'badge-no'}`}>
                  {modelInfo.capabilities.reasoning ? 'Yes' : 'No'}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Tooling</label>
                <span className={`badge ${modelInfo.capabilities.tooling ? 'badge-yes' : 'badge-no'}`}>
                  {modelInfo.capabilities.tooling ? 'Yes' : 'No'}
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
                  {modelInfo.files.model.url || '-'} {modelInfo.files.model.size && `(${modelInfo.files.model.size})`}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Projection</label>
                <span>
                  {modelInfo.files.proj.url || '-'} {modelInfo.files.proj.size && `(${modelInfo.files.proj.size})`}
                </span>
              </div>
              <div className="model-meta-item">
                <label>Jinja</label>
                <span>
                  {modelInfo.files.jinja.url || '-'} {modelInfo.files.jinja.size && `(${modelInfo.files.jinja.size})`}
                </span>
              </div>
            </div>
          </div>

          {modelInfo.metadata.description && (
            <div style={{ marginTop: '24px' }}>
              <h4 style={{ marginBottom: '12px' }}>Description</h4>
              <p>{modelInfo.metadata.description}</p>
            </div>
          )}

          <div style={{ marginTop: '24px' }}>
            <h4 style={{ marginBottom: '12px' }}>Metadata</h4>
            <div className="model-meta">
              <div className="model-meta-item">
                <label>Created</label>
                <span>{new Date(modelInfo.metadata.created).toLocaleString()}</span>
              </div>
              <div className="model-meta-item">
                <label>Collections</label>
                <span>{modelInfo.metadata.collections || '-'}</span>
              </div>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}
