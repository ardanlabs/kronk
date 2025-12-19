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

export default function ModelShow() {
  const { models, loading: listLoading, error: listError, loadModels, invalidate } = useModelList();
  const [selectedModel, setSelectedModel] = useState('');
  const [data, setData] = useState<ModelInfoResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  const handleShow = async () => {
    if (!selectedModel) return;

    setLoading(true);
    setError(null);
    setData(null);
    try {
      const response = await api.showModel(selectedModel);
      setData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load model info');
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Show Model</h2>
        <p>View detailed information about a model</p>
      </div>

      <div className="card">
        {listLoading ? (
          <div className="loading">Loading models</div>
        ) : (
          <>
            <div className="form-group">
              <label htmlFor="modelSelect">Select Model</label>
              <select
                id="modelSelect"
                value={selectedModel}
                onChange={(e) => setSelectedModel(e.target.value)}
                disabled={loading}
              >
                <option value="">-- Select a model --</option>
                {models?.data?.map((model) => (
                  <option key={model.id} value={model.id}>
                    {model.id}
                  </option>
                ))}
              </select>
            </div>

            <div style={{ display: 'flex', gap: '12px' }}>
              <button
                className="btn btn-primary"
                onClick={handleShow}
                disabled={!selectedModel || loading}
              >
                {loading ? 'Loading...' : 'Show Model'}
              </button>
              <button
                className="btn btn-secondary"
                onClick={() => {
                  invalidate();
                  loadModels();
                }}
                disabled={listLoading || loading}
              >
                Refresh List
              </button>
            </div>
          </>
        )}
      </div>

      {(error || listError) && <div className="alert alert-error">{error || listError}</div>}

      {data && (
        <div className="card">
          <h3 style={{ marginBottom: '16px' }}>{data.id}</h3>

          <div className="model-meta">
            <div className="model-meta-item">
              <label>Owner</label>
              <span>{data.owned_by}</span>
            </div>
            <div className="model-meta-item">
              <label>Size</label>
              <span>{formatBytes(data.size)}</span>
            </div>
            <div className="model-meta-item">
              <label>Created</label>
              <span>{new Date(data.created).toLocaleString()}</span>
            </div>
            <div className="model-meta-item">
              <label>Has Projection</label>
              <span className={`badge ${data.has_projection ? 'badge-yes' : 'badge-no'}`}>
                {data.has_projection ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Has Encoder</label>
              <span className={`badge ${data.has_encoder ? 'badge-yes' : 'badge-no'}`}>
                {data.has_encoder ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Has Decoder</label>
              <span className={`badge ${data.has_decoder ? 'badge-yes' : 'badge-no'}`}>
                {data.has_decoder ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Is Recurrent</label>
              <span className={`badge ${data.is_recurrent ? 'badge-yes' : 'badge-no'}`}>
                {data.is_recurrent ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Is Hybrid</label>
              <span className={`badge ${data.is_hybrid ? 'badge-yes' : 'badge-no'}`}>
                {data.is_hybrid ? 'Yes' : 'No'}
              </span>
            </div>
            <div className="model-meta-item">
              <label>Is GPT</label>
              <span className={`badge ${data.is_gpt ? 'badge-yes' : 'badge-no'}`}>
                {data.is_gpt ? 'Yes' : 'No'}
              </span>
            </div>
          </div>

          {data.desc && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                Description
              </label>
              <p>{data.desc}</p>
            </div>
          )}

          {data.metadata && Object.keys(data.metadata).length > 0 && (
            <div style={{ marginTop: '16px' }}>
              <label style={{ fontWeight: 500, display: 'block', marginBottom: '8px' }}>
                Metadata
              </label>
              <div className="model-meta">
                {Object.entries(data.metadata).map(([key, value]) => (
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
