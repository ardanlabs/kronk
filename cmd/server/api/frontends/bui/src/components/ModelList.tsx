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

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  const handleRowClick = async (modelId: string) => {
    if (selectedModelId === modelId) {
      setSelectedModelId(null);
      setModelInfo(null);
      return;
    }

    setSelectedModelId(modelId);
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

  return (
    <div>
      <div className="page-header">
        <h2>Models</h2>
        <p>List of all models available in the system. Click a model to view details.</p>
      </div>

      <div className="card">
        {loading && <div className="loading">Loading models</div>}

        {error && <div className="alert alert-error">{error}</div>}

        {!loading && !error && models && (
          <div className="table-container">
            {models.data && models.data.length > 0 ? (
              <table>
                <thead>
                  <tr>
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

        <div style={{ marginTop: '16px' }}>
          <button
            className="btn btn-secondary"
            onClick={() => {
              invalidate();
              loadModels();
              setSelectedModelId(null);
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

          {modelInfo.metadata && Object.keys(modelInfo.metadata).length > 0 && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                Metadata
              </label>
              <div className="model-meta">
                {Object.entries(modelInfo.metadata).map(([key, value]) => (
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
