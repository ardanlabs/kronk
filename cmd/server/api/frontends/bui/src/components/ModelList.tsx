import { useState, useEffect } from 'react';
import { api } from '../services/api';
import { useModelList } from '../contexts/ModelListContext';
import type { ModelInfoResponse } from '../types';

function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
}

function formatDate(dateStr: string): string {
  return new Date(dateStr).toLocaleString();
}

export default function ModelList() {
  const { models, loading, error, loadModels, invalidate } = useModelList();
  const [selectedModelId, setSelectedModelId] = useState<string | null>(null);
  const [modelInfo, setModelInfo] = useState<ModelInfoResponse | null>(null);
  const [infoLoading, setInfoLoading] = useState(false);
  const [infoError, setInfoError] = useState<string | null>(null);
  const [rebuildingIndex, setRebuildingIndex] = useState(false);
  const [rebuildError, setRebuildError] = useState<string | null>(null);
  const [rebuildSuccess, setRebuildSuccess] = useState(false);

  const [confirmingRemove, setConfirmingRemove] = useState(false);
  const [removing, setRemoving] = useState(false);
  const [removeError, setRemoveError] = useState<string | null>(null);
  const [removeSuccess, setRemoveSuccess] = useState<string | null>(null);

  const handleRebuildIndex = async () => {
    setRebuildingIndex(true);
    setRebuildError(null);
    setRebuildSuccess(false);
    try {
      await api.rebuildModelIndex();
      invalidate();
      loadModels();
      setSelectedModelId(null);
      setModelInfo(null);
      setRebuildSuccess(true);
      setTimeout(() => setRebuildSuccess(false), 3000);
    } catch (err) {
      setRebuildError(err instanceof Error ? err.message : 'Failed to rebuild index');
    } finally {
      setRebuildingIndex(false);
    }
  };

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  const handleRowClick = async (modelId: string) => {
    if (selectedModelId === modelId) {
      setSelectedModelId(null);
      setModelInfo(null);
      setConfirmingRemove(false);
      return;
    }

    setSelectedModelId(modelId);
    setConfirmingRemove(false);
    setRemoveError(null);
    setRemoveSuccess(null);
    setInfoLoading(true);
    setInfoError(null);
    setModelInfo(null);

    try {
      const response = await api.showModel(modelId);
      setModelInfo(response);
    } catch (err) {
      setInfoError(err instanceof Error ? err.message : 'Failed to load model info');
    } finally {
      setInfoLoading(false);
    }
  };

  const handleRemoveClick = () => {
    if (!selectedModelId) return;
    setConfirmingRemove(true);
  };

  const handleConfirmRemove = async () => {
    if (!selectedModelId) return;

    setRemoving(true);
    setConfirmingRemove(false);
    setRemoveError(null);
    setRemoveSuccess(null);

    try {
      await api.removeModel(selectedModelId);
      setRemoveSuccess(`Model "${selectedModelId}" removed successfully`);
      setSelectedModelId(null);
      setModelInfo(null);
      invalidate();
      await loadModels();
      setTimeout(() => setRemoveSuccess(null), 3000);
    } catch (err) {
      setRemoveError(err instanceof Error ? err.message : 'Failed to remove model');
    } finally {
      setRemoving(false);
    }
  };

  const handleCancelRemove = () => {
    setConfirmingRemove(false);
  };

  return (
    <div>
      <div className="page-header">
        <h2>Models</h2>
        <p>List of all models available in the system. Click a model to view details.</p>
      </div>

      <div className="card">
        {loading && <div className="loading">Loading models</div>}

        {error && <div className="alert alert-error">{error}</div>}
        {removeError && <div className="alert alert-error">{removeError}</div>}
        {removeSuccess && <div className="alert alert-success">{removeSuccess}</div>}

        {!loading && !error && models && (
          <div className="table-container">
            {models.data && models.data.length > 0 ? (
              <table>
                <thead>
                  <tr>
                    <th style={{ width: '40px', textAlign: 'center' }} title="Validated">✓</th>
                    <th>ID</th>
                    <th>Owner</th>
                    <th>Family</th>
                    <th>Size</th>
                    <th>Modified</th>
                  </tr>
                </thead>
                <tbody>
                  {(() => {
                    const mainModels = models.data.filter((m) => !m.id.includes('/'));
                    const extensionModels = models.data.filter((m) => m.id.includes('/'));

                    return mainModels.map((model) => {
                      const extensions = extensionModels.filter((ext) => ext.id.startsWith(model.id + '/'));
                      const isParentSelected = selectedModelId === model.id;
                      const isExtensionSelected = selectedModelId?.startsWith(model.id + '/');
                      const showExtensions = isParentSelected || isExtensionSelected;
                      return (
                        <>
                          <tr
                            key={model.id}
                            onClick={() => handleRowClick(model.id)}
                            className={selectedModelId === model.id ? 'selected' : ''}
                            style={{ cursor: 'pointer' }}
                          >
                            <td style={{ textAlign: 'center', color: model.validated ? 'inherit' : '#e74c3c' }}>{model.validated ? '✓' : '✗'}</td>
                            <td>{model.id}</td>
                            <td>{model.owned_by}</td>
                            <td>{model.model_family}</td>
                            <td>{formatBytes(model.size)}</td>
                            <td>{formatDate(model.modified)}</td>
                          </tr>
                          {showExtensions && extensions.map((ext) => (
                            <tr
                              key={ext.id}
                              onClick={() => handleRowClick(ext.id)}
                              className={selectedModelId === ext.id ? 'selected' : ''}
                              style={{ cursor: 'pointer' }}
                            >
                              <td></td>
                              <td style={{ paddingLeft: '24px' }}>↳ {ext.id}</td>
                              <td></td>
                              <td>Extension Model</td>
                              <td></td>
                              <td></td>
                            </tr>
                          ))}
                        </>
                      );
                    });
                  })()}
                </tbody>
              </table>
            ) : (
              <div className="empty-state">
                <h3>No models found</h3>
                <p>Pull a model to get started</p>
              </div>
            )}
          </div>
        )}

        <div style={{ marginTop: '16px', display: 'flex', gap: '8px' }}>
          <button
            className="btn btn-secondary"
            onClick={() => {
              invalidate();
              loadModels();
              setSelectedModelId(null);
              setModelInfo(null);
              setConfirmingRemove(false);
              setRemoveError(null);
              setRemoveSuccess(null);
              setInfoError(null);
              setRebuildError(null);
              setRebuildSuccess(false);
            }}
            disabled={loading}
          >
            Refresh
          </button>
          <button
            className="btn btn-secondary"
            onClick={handleRebuildIndex}
            disabled={rebuildingIndex || loading}
          >
            {rebuildingIndex ? 'Rebuilding...' : 'Rebuild Index'}
          </button>
          {selectedModelId && !confirmingRemove && (
            <button
              className="btn btn-danger"
              onClick={handleRemoveClick}
              disabled={removing}
            >
              Remove Model
            </button>
          )}
          {selectedModelId && confirmingRemove && (
            <>
              <button className="btn btn-danger" onClick={handleConfirmRemove} disabled={removing}>
                {removing ? 'Removing...' : 'Yes, Remove'}
              </button>
              <button className="btn btn-secondary" onClick={handleCancelRemove} disabled={removing}>
                Cancel
              </button>
            </>
          )}
        </div>
        {rebuildError && <div className="alert alert-error" style={{ marginTop: '8px' }}>{rebuildError}</div>}
        {rebuildSuccess && <div className="alert alert-success" style={{ marginTop: '8px' }}>Index rebuilt successfully</div>}
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

          {modelInfo.model_config && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                Model Configuration
              </label>
              <div className="model-meta">
                <div className="model-meta-item">
                  <label>Device</label>
                  <span>{modelInfo.model_config.devices?.length ? modelInfo.model_config.devices.join(', ') : modelInfo.model_config.device || 'default'}</span>
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
                <div className="model-meta-item">
                  <label>Owner</label>
                  <span>{modelInfo.owned_by}</span>
                </div>
                <div className="model-meta-item">
                  <label>Size</label>
                  <span>{formatBytes(modelInfo.size)}</span>
                </div>
                <div className="model-meta-item">
                  <label>Created</label>
                  <span>{new Date(modelInfo.created).toLocaleString()}</span>
                </div>
                <div className="model-meta-item">
                  <label>Has Projection</label>
                  <span className={`badge ${modelInfo.has_projection ? 'badge-yes' : 'badge-no'}`}>
                    {modelInfo.has_projection ? 'Yes' : 'No'}
                  </span>
                </div>
                <div className="model-meta-item">
                  <label>Is GPT</label>
                  <span className={`badge ${modelInfo.is_gpt ? 'badge-yes' : 'badge-no'}`}>
                    {modelInfo.is_gpt ? 'Yes' : 'No'}
                  </span>
                </div>
              </div>
            </div>
          )}

          {modelInfo.model_config?.['sampling-parameters'] && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                Sampling Parameters
              </label>
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

          {modelInfo.desc && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                Description
              </label>
              <p>{modelInfo.desc}</p>
            </div>
          )}

          {modelInfo.vram && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                VRAM Requirements
              </label>
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

          {modelInfo.metadata && Object.keys(modelInfo.metadata).filter(k => k !== 'tokenizer.chat_template').length > 0 && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                Metadata
              </label>
              <div className="model-meta">
                {Object.entries(modelInfo.metadata)
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

          {modelInfo.metadata?.['tokenizer.chat_template'] && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                tokenizer.chat_template
              </label>
              <pre style={{
                background: 'var(--color-gray-100)',
                padding: '12px',
                borderRadius: '6px',
                fontSize: '12px',
                overflow: 'auto',
                whiteSpace: 'pre-wrap',
                wordBreak: 'break-word'
              }}>
                {modelInfo.metadata['tokenizer.chat_template']}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  );
}
