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
                  {models.data.map((model) => (
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
                  ))}
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

          <div className="model-meta">
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
              <label>Has Encoder</label>
              <span className={`badge ${modelInfo.has_encoder ? 'badge-yes' : 'badge-no'}`}>
                {modelInfo.has_encoder ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Has Decoder</label>
              <span className={`badge ${modelInfo.has_decoder ? 'badge-yes' : 'badge-no'}`}>
                {modelInfo.has_decoder ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Is Recurrent</label>
              <span className={`badge ${modelInfo.is_recurrent ? 'badge-yes' : 'badge-no'}`}>
                {modelInfo.is_recurrent ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Is Hybrid</label>
              <span className={`badge ${modelInfo.is_hybrid ? 'badge-yes' : 'badge-no'}`}>
                {modelInfo.is_hybrid ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Is GPT</label>
              <span className={`badge ${modelInfo.is_gpt ? 'badge-yes' : 'badge-no'}`}>
                {modelInfo.is_gpt ? 'Yes' : 'No'}
              </span>
            </div>
          </div>

          {modelInfo.desc && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                Description
              </label>
              <p>{modelInfo.desc}</p>
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
        </div>
      )}
    </div>
  );
}
