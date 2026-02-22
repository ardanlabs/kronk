import { useState, useEffect } from 'react';
import { api } from '../services/api';
import type { ModelDetailsResponse } from '../types';

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

export default function ModelPs() {
  const [data, setData] = useState<ModelDetailsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [unloading, setUnloading] = useState<string | null>(null);

  useEffect(() => {
    loadRunningModels();
  }, []);

  const loadRunningModels = async () => {
    setLoading(true);
    setError(null);
    try {
      const response = await api.listRunningModels();
      setData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load running models');
    } finally {
      setLoading(false);
    }
  };

  const handleUnload = async (modelId: string) => {
    if (!confirm(`Unload model "${modelId}"?`)) return;
    setUnloading(modelId);
    setError(null);
    try {
      await api.unloadModel(modelId);
      await loadRunningModels();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to unload model');
    } finally {
      setUnloading(null);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Running Models</h2>
        <p>Models currently loaded in cache</p>
      </div>

      <div className="card">
        {loading && <div className="loading">Loading running models</div>}

        {error && <div className="alert alert-error">{error}</div>}

        {!loading && !error && data && (
          <div className="table-container">
            {data.length > 0 ? (
              <table>
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Owner</th>
                    <th>Family</th>
                    <th style={{ textAlign: 'right' }}>Size</th>
                    <th style={{ textAlign: 'right' }}>VRAM Total</th>
                    <th style={{ textAlign: 'right' }}>Slot Memory</th>
                    <th>Expires At</th>
                    <th>Active Streams</th>
                    <th></th>
                  </tr>
                </thead>
                <tbody>
                  {data.map((model) => (
                    <tr key={model.id}>
                      <td>{model.id}</td>
                      <td>{model.owned_by}</td>
                      <td>{model.model_family}</td>
                      <td style={{ textAlign: 'right' }}>{formatBytes(model.size)}</td>
                      <td style={{ textAlign: 'right' }}>{formatBytes(model.vram_total)}</td>
                      <td style={{ textAlign: 'right' }}>{formatBytes(model.slot_memory)}</td>
                      <td>{formatDate(model.expires_at)}</td>
                      <td>{model.active_streams}</td>
                      <td>
                        <button
                          className="btn btn-danger btn-sm"
                          onClick={() => handleUnload(model.id)}
                          disabled={unloading === model.id || model.active_streams > 0}
                          title={model.active_streams > 0 ? 'Cannot unload while streams are active' : 'Unload model from cache'}
                        >
                          {unloading === model.id ? 'Unloading…' : 'Unload'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
                {data.length > 1 && (
                  <tfoot>
                    <tr>
                      <td colSpan={4} style={{ textAlign: 'right', fontWeight: 'bold' }}>Total:</td>
                      <td style={{ textAlign: 'right', fontWeight: 'bold' }}>{formatBytes(data.reduce((sum, m) => sum + m.vram_total, 0))}</td>
                      <td style={{ textAlign: 'right', fontWeight: 'bold' }}>{formatBytes(data.reduce((sum, m) => sum + m.slot_memory, 0))}</td>
                      <td colSpan={3}></td>
                    </tr>
                  </tfoot>
                )}
              </table>
            ) : (
              <div className="empty-state">
                <h3>No running models</h3>
                <p>Models will appear here when loaded into cache</p>
              </div>
            )}
          </div>
        )}

        <div style={{ marginTop: '16px' }}>
          <button className="btn btn-secondary" onClick={loadRunningModels} disabled={loading}>
            Refresh
          </button>
        </div>
      </div>
    </div>
  );
}
