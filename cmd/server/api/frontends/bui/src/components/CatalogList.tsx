import { useState, useEffect, useRef } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { CatalogModelResponse, CatalogModelsResponse, PullResponse } from '../types';

type DetailTab = 'details' | 'pull';

export default function CatalogList() {
  const { invalidate } = useModelList();
  const [data, setData] = useState<CatalogModelsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [modelInfo, setModelInfo] = useState<CatalogModelResponse | null>(null);
  const [infoLoading, setInfoLoading] = useState(false);
  const [infoError, setInfoError] = useState<string | null>(null);

  const [activeTab, setActiveTab] = useState<DetailTab>('details');
  const [pulling, setPulling] = useState(false);
  const [pullMessages, setPullMessages] = useState<Array<{ text: string; type: 'info' | 'error' | 'success' }>>([]);
  const closeRef = useRef<(() => void) | null>(null);

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
      setActiveTab('details');
      setPullMessages([]);
      return;
    }

    setSelectedId(id);
    setActiveTab('details');
    setPullMessages([]);
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

  const handlePull = () => {
    if (!selectedId) return;

    setPulling(true);
    setPullMessages([]);
    setActiveTab('pull');

    const ANSI_INLINE = '\x1b[1A\r\x1b[K';

    const addMessage = (text: string, type: 'info' | 'error' | 'success') => {
      setPullMessages((prev) => [...prev, { text, type }]);
    };

    const updateLastMessage = (text: string, type: 'info' | 'error' | 'success') => {
      setPullMessages((prev) => {
        if (prev.length === 0) {
          return [{ text, type }];
        }
        const updated = [...prev];
        updated[updated.length - 1] = { text, type };
        return updated;
      });
    };

    closeRef.current = api.pullCatalogModel(
      selectedId,
      (data: PullResponse) => {
        if (data.status) {
          if (data.status.startsWith(ANSI_INLINE)) {
            const cleanText = data.status.slice(ANSI_INLINE.length);
            updateLastMessage(cleanText, 'info');
          } else {
            addMessage(data.status, 'info');
          }
        }
        if (data.model_file) {
          addMessage(`Model file: ${data.model_file}`, 'info');
        }

      },
      (errorMsg: string) => {
        addMessage(errorMsg, 'error');
        setPulling(false);
      },
      () => {
        addMessage('Pull complete!', 'success');
        setPulling(false);
        invalidate();
        loadCatalog();
      }
    );
  };

  const handleCancelPull = () => {
    if (closeRef.current) {
      closeRef.current();
      closeRef.current = null;
    }
    setPulling(false);
    setPullMessages((prev) => [...prev, { text: 'Cancelled', type: 'error' }]);
  };

  const isDownloaded = data?.find((m) => m.id === selectedId)?.downloaded ?? false;

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
                    <th style={{ width: '40px', textAlign: 'center' }} title="Validated">✓</th>
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
                      <td style={{ textAlign: 'center', color: model.validated ? 'inherit' : '#e74c3c' }}>{model.validated ? '✓' : '✗'}</td>
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

        <div style={{ marginTop: '16px', display: 'flex', gap: '8px' }}>
          <button
            className="btn btn-secondary"
            onClick={() => {
              loadCatalog();
              setSelectedId(null);
              setModelInfo(null);
              setPullMessages([]);
              setActiveTab('details');
              setInfoError(null);
            }}
            disabled={loading}
          >
            Refresh
          </button>
          {selectedId && (
            <button
              className="btn btn-primary"
              onClick={handlePull}
              disabled={pulling || isDownloaded}
            >
              {pulling ? 'Pulling...' : isDownloaded ? 'Already Downloaded' : 'Pull Model'}
            </button>
          )}
          {pulling && (
            <button
              className="btn btn-danger"
              onClick={handleCancelPull}
            >
              Cancel
            </button>
          )}
        </div>
      </div>

      {infoError && <div className="alert alert-error">{infoError}</div>}

      {infoLoading && (
        <div className="card">
          <div className="loading">Loading model details</div>
        </div>
      )}

      {selectedId && !infoLoading && (modelInfo || pullMessages.length > 0) && (
        <div className="card">
          <div className="tabs">
            <button
              className={`tab ${activeTab === 'details' ? 'active' : ''}`}
              onClick={() => setActiveTab('details')}
            >
              Details
            </button>
            <button
              className={`tab ${activeTab === 'pull' ? 'active' : ''}`}
              onClick={() => setActiveTab('pull')}
              disabled={pullMessages.length === 0 && !pulling}
            >
              Pull Output
            </button>
          </div>

          {activeTab === 'details' && modelInfo && (
            <>
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
                  <label>Gated Model</label>
                  <span className={`badge ${modelInfo.gated_model ? 'badge-yes' : 'badge-no'}`}>
                    {modelInfo.gated_model ? 'Yes' : 'No'}
                  </span>
                </div>
                <div className="model-meta-item">
                  <label>Endpoint</label>
                  <span>{modelInfo.capabilities.endpoint}</span>
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
                <h4 style={{ marginBottom: '12px' }}>Web Page</h4>
                <div className="model-meta-item">
                  <span>
                    {modelInfo.web_page ? (
                      <a href={modelInfo.web_page} target="_blank" rel="noopener noreferrer">
                        {modelInfo.web_page}
                      </a>
                    ) : '-'}
                  </span>
                </div>
              </div>

              <div style={{ marginTop: '24px' }}>
                <h4 style={{ marginBottom: '12px' }}>Files</h4>
                <div className="model-meta-item" style={{ marginBottom: '12px' }}>
                  <label>Model URL</label>
                  <span>
                    {modelInfo.files.model.length > 0 ? (
                      modelInfo.files.model.map((file, idx) => (
                        <div key={idx}>{file.url} {file.size && `(${file.size})`}</div>
                      ))
                    ) : '-'}
                  </span>
                </div>
                <div className="model-meta-item" style={{ marginBottom: '12px' }}>
                  <label>Projection URL</label>
                  <span>
                    {modelInfo.files.proj.length > 0 ? (
                      modelInfo.files.proj.map((file, idx) => (
                        <div key={idx}>{file.url} {file.size && `(${file.size})`}</div>
                      ))
                    ) : '-'}
                  </span>
                </div>
                <div className="model-meta-item">
                  <label>Template File</label>
                  <span>{modelInfo.template || '-'}</span>
                </div>
              </div>

              {modelInfo.metadata.description && (
                <div style={{ marginTop: '24px' }}>
                  <h4 style={{ marginBottom: '12px' }}>Description</h4>
                  <div className="model-meta-item">
                    <span>{modelInfo.metadata.description}</span>
                  </div>
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
            </>
          )}

          {activeTab === 'pull' && (
            <div>
              <h3 style={{ marginBottom: '16px' }}>Pull Output: {selectedId}</h3>
              {pullMessages.length > 0 ? (
                <div className="status-box">
                  {pullMessages.map((msg, idx) => (
                    <div key={idx} className={`status-line ${msg.type}`}>
                      {msg.text}
                    </div>
                  ))}
                </div>
              ) : (
                <p>No pull output yet.</p>
              )}
              {pulling && (
                <button
                  className="btn btn-danger"
                  onClick={handleCancelPull}
                  style={{ marginTop: '16px' }}
                >
                  Cancel
                </button>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
