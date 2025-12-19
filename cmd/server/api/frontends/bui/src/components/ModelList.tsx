import { useEffect } from 'react';
import { useModelList } from '../contexts/ModelListContext';

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

  useEffect(() => {
    loadModels();
  }, [loadModels]);

  return (
    <div>
      <div className="page-header">
        <h2>Models</h2>
        <p>List of all models available in the system</p>
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
                    <tr key={model.id}>
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
            }}
            disabled={loading}
          >
            Refresh
          </button>
        </div>
      </div>
    </div>
  );
}
