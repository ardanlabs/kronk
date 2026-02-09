import { useState, useEffect } from 'react';
import { useNavigate } from 'react-router-dom';
import { api } from '../services/api';
import { useDownload } from '../contexts/DownloadContext';
import type { CatalogModelResponse, CatalogModelsResponse } from '../types';

type DetailTab = 'details' | 'pull';

function formatBytes(bytes: number): string {
  const KB = 1024;
  const MB = KB * 1024;
  const GB = MB * 1024;

  if (bytes >= GB) {
    return `${(bytes / GB).toFixed(2)} GB`;
  } else if (bytes >= MB) {
    return `${(bytes / MB).toFixed(2)} MB`;
  } else if (bytes >= KB) {
    return `${(bytes / KB).toFixed(2)} KB`;
  }
  return `${bytes} bytes`;
}

export default function CatalogList() {
  const navigate = useNavigate();
  const { download, startCatalogDownload, cancelDownload } = useDownload();
  const [data, setData] = useState<CatalogModelsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [modelInfo, setModelInfo] = useState<CatalogModelResponse | null>(null);
  const [infoLoading, setInfoLoading] = useState(false);
  const [infoError, setInfoError] = useState<string | null>(null);

  const [activeTab, setActiveTab] = useState<DetailTab>('details');

  const isCatalogDownload = download?.kind === 'catalog' && download.catalogId === selectedId;
  const pulling = isCatalogDownload ? download.status === 'downloading' : false;
  const pullMessages = isCatalogDownload ? download.messages : [];
  const hasCatalogPullActivity = download?.kind === 'catalog' && (download.status === 'downloading' || download.messages.length > 0);

  useEffect(() => {
    loadCatalog();
  }, []);

  useEffect(() => {
    if (download?.kind === 'catalog' && download.catalogId) {
      if (download.status === 'downloading') {
        setSelectedId(download.catalogId);
        setActiveTab('pull');
      }
      if (download.status === 'complete') {
        loadCatalog();
      }
    }
  }, [download?.status]);

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
      return;
    }

    setSelectedId(id);
    setActiveTab('details');
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
    startCatalogDownload(selectedId);
    setActiveTab('pull');
  };

  const handleCancelPull = () => {
    cancelDownload();
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
                        {model.capabilities.embedding && (
                          <span className="badge badge-yes" style={{ marginRight: 4 }}>
                            Embedding
                          </span>
                        )}
                        {model.capabilities.rerank && (
                          <span className="badge badge-yes" style={{ marginRight: 4 }}>
                            Rerank
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
              setActiveTab('details');
              setInfoError(null);
            }}
            disabled={loading}
          >
            Refresh
          </button>
          {selectedId && (
            <>
              <button
                className="btn btn-primary"
                onClick={handlePull}
                disabled={pulling || isDownloaded}
              >
                {pulling ? 'Pulling...' : isDownloaded ? 'Already Downloaded' : 'Pull Model'}
              </button>
              <button
                className="btn btn-secondary"
                onClick={() => navigate(`/catalog/editor?id=${encodeURIComponent(selectedId)}`)}
              >
                Edit
              </button>
            </>
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

      {selectedId && !infoLoading && (modelInfo || hasCatalogPullActivity) && (
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
                  <div className="model-meta-item">
                    <label>Embedding</label>
                    <span className={`badge ${modelInfo.capabilities.embedding ? 'badge-yes' : 'badge-no'}`}>
                      {modelInfo.capabilities.embedding ? 'Yes' : 'No'}
                    </span>
                  </div>
                  <div className="model-meta-item">
                    <label>Rerank</label>
                    <span className={`badge ${modelInfo.capabilities.rerank ? 'badge-yes' : 'badge-no'}`}>
                      {modelInfo.capabilities.rerank ? 'Yes' : 'No'}
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
                    {modelInfo.files.proj.url ? (
                      <div>{modelInfo.files.proj.url} {modelInfo.files.proj.size && `(${modelInfo.files.proj.size})`}</div>
                    ) : '-'}
                  </span>
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
                <h4 style={{ marginBottom: '12px' }}>Catalog Metadata</h4>
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

              {modelInfo.vram && (
                <div style={{ marginTop: '24px' }}>
                  <h4 style={{ marginBottom: '12px' }}>VRAM Requirements</h4>
                  <div className="model-meta">
                    <div className="model-meta-item">
                      <label>Total VRAM</label>
                      <span>{formatBytes(modelInfo.vram.total_vram)}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Slot Memory (KV Cache)</label>
                      <span>{formatBytes(modelInfo.vram.slot_memory)}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>KV Per Slot</label>
                      <span>{formatBytes(modelInfo.vram.kv_per_slot)}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Total Slots</label>
                      <span>{modelInfo.vram.total_slots}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>KV Per Token/Layer</label>
                      <span>{formatBytes(modelInfo.vram.kv_per_token_per_layer)}</span>
                    </div>
                  </div>
                  <div style={{ marginTop: '2rem' }} />
                  <div className="model-meta">
                    <div className="model-meta-item">
                      <label>Model Size</label>
                      <span>{formatBytes(modelInfo.vram.input.model_size_bytes)}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Block Count (Layers)</label>
                      <span>{modelInfo.vram.input.block_count}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Head Count KV</label>
                      <span>{modelInfo.vram.input.head_count_kv}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Key Length</label>
                      <span>{modelInfo.vram.input.key_length}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Value Length</label>
                      <span>{modelInfo.vram.input.value_length}</span>
                    </div>
                  </div>
                </div>
              )}

              {modelInfo.model_config && (
                <div style={{ marginTop: '24px' }}>
                  <h4 style={{ marginBottom: '12px' }}>Model Configuration</h4>
                  <div className="model-meta">
                    <div className="model-meta-item">
                      <label>Device</label>
                      <span>{modelInfo.model_config.device || 'default'}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Context Window</label>
                      <span>{modelInfo.model_config['context-window']}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Batch Size</label>
                      <span>{modelInfo.model_config.nbatch}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Micro Batch Size</label>
                      <span>{modelInfo.model_config.nubatch}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Threads</label>
                      <span>{modelInfo.model_config.nthreads}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Batch Threads</label>
                      <span>{modelInfo.model_config['nthreads-batch']}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Cache Type K</label>
                      <span>{modelInfo.model_config['cache-type-k'] || 'default'}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Cache Type V</label>
                      <span>{modelInfo.model_config['cache-type-v'] || 'default'}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Flash Attention</label>
                      <span>{modelInfo.model_config['flash-attention'] || 'default'}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Max Sequences</label>
                      <span>{modelInfo.model_config['nseq-max']}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>GPU Layers</label>
                      <span>{modelInfo.model_config['ngpu-layers'] ?? 'auto'}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Split Mode</label>
                      <span>{modelInfo.model_config['split-mode'] || 'default'}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>System Prompt Cache</label>
                      <span className={`badge ${modelInfo.model_config['system-prompt-cache'] ? 'badge-yes' : 'badge-no'}`}>
                        {modelInfo.model_config['system-prompt-cache'] ? 'Yes' : 'No'}
                      </span>
                    </div>
                    <div className="model-meta-item">
                      <label>Incremental Cache</label>
                      <span className={`badge ${modelInfo.model_config['incremental-cache'] ? 'badge-yes' : 'badge-no'}`}>
                        {modelInfo.model_config['incremental-cache'] ? 'Yes' : 'No'}
                      </span>
                    </div>
                    {(modelInfo.model_config['system-prompt-cache'] || modelInfo.model_config['incremental-cache']) && (
                      <div className="model-meta-item">
                        <label>Max Cache Sessions</label>
                        <span>{modelInfo.model_config['max-cache-sessions'] || 1}</span>
                      </div>
                    )}
                    {!!modelInfo.model_config['rope-scaling-type'] && modelInfo.model_config['rope-scaling-type'] !== 'none' && (
                      <>
                        <div className="model-meta-item">
                          <label>RoPE Scaling</label>
                          <span>{modelInfo.model_config['rope-scaling-type']}</span>
                        </div>
                        <div className="model-meta-item">
                          <label>YaRN Original Context</label>
                          <span>{modelInfo.model_config['yarn-orig-ctx'] ?? 'auto'}</span>
                        </div>
                        {modelInfo.model_config['rope-freq-base'] != null && (
                          <div className="model-meta-item">
                            <label>RoPE Freq Base</label>
                            <span>{modelInfo.model_config['rope-freq-base']}</span>
                          </div>
                        )}
                        {modelInfo.model_config['yarn-ext-factor'] != null && (
                          <div className="model-meta-item">
                            <label>YaRN Ext Factor</label>
                            <span>{modelInfo.model_config['yarn-ext-factor']}</span>
                          </div>
                        )}
                        {modelInfo.model_config['yarn-attn-factor'] != null && (
                          <div className="model-meta-item">
                            <label>YaRN Attn Factor</label>
                            <span>{modelInfo.model_config['yarn-attn-factor']}</span>
                          </div>
                        )}
                      </>
                    )}
                  </div>
                </div>
              )}

              {modelInfo.model_config?.['sampling-parameters'] && (
                <div style={{ marginTop: '24px' }}>
                  <h4 style={{ marginBottom: '12px' }}>Sampling Parameters</h4>
                  <div className="model-meta">
                    <div className="model-meta-item">
                      <label>Temperature</label>
                      <span>{modelInfo.model_config['sampling-parameters'].temperature.toFixed(2)}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Top K</label>
                      <span>{modelInfo.model_config['sampling-parameters'].top_k}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Top P</label>
                      <span>{modelInfo.model_config['sampling-parameters'].top_p.toFixed(2)}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Min P</label>
                      <span>{modelInfo.model_config['sampling-parameters'].min_p.toFixed(2)}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Max Tokens</label>
                      <span>{modelInfo.model_config['sampling-parameters'].max_tokens}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Repeat Penalty</label>
                      <span>{modelInfo.model_config['sampling-parameters'].repeat_penalty.toFixed(2)}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Repeat Last N</label>
                      <span>{modelInfo.model_config['sampling-parameters'].repeat_last_n}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Enable Thinking</label>
                      <span>{modelInfo.model_config['sampling-parameters'].enable_thinking || 'default'}</span>
                    </div>
                    <div className="model-meta-item">
                      <label>Reasoning Effort</label>
                      <span>{modelInfo.model_config['sampling-parameters'].reasoning_effort || 'default'}</span>
                    </div>
                  </div>
                </div>
              )}

              {modelInfo.model_metadata && Object.keys(modelInfo.model_metadata).filter(k => k !== 'tokenizer.chat_template').length > 0 && (
                <div style={{ marginTop: '24px' }}>
                  <h4 style={{ marginBottom: '12px' }}>Model Metadata</h4>
                  <div className="model-meta">
                    {Object.entries(modelInfo.model_metadata)
                      .filter(([key]) => key !== 'tokenizer.chat_template')
                      .map(([key, value]) => (
                      <div key={key} className="model-meta-item">
                        <label>{key}</label>
                        <span>{value}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}

              {modelInfo.model_metadata?.['tokenizer.chat_template'] && (
                <div style={{ marginTop: '24px' }}>
                  <h4 style={{ marginBottom: '12px' }}>Template</h4>
                  <pre style={{
                    background: 'var(--color-gray-100)',
                    padding: '12px',
                    borderRadius: '6px',
                    fontSize: '12px',
                    overflow: 'auto',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word'
                  }}>
                    {modelInfo.model_metadata['tokenizer.chat_template']}
                  </pre>
                </div>
              )}
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
