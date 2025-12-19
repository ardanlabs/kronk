import { useState, useEffect } from 'react';
import { api } from '../services/api';
import type { CatalogModelsResponse } from '../types';

export default function CatalogList() {
  const [data, setData] = useState<CatalogModelsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

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

  return (
    <div>
      <div className="page-header">
        <h2>Catalog</h2>
        <p>Browse available models in the catalog</p>
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
                    <tr key={model.id}>
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
          <button className="btn btn-secondary" onClick={loadCatalog} disabled={loading}>
            Refresh
          </button>
        </div>
      </div>
    </div>
  );
}
