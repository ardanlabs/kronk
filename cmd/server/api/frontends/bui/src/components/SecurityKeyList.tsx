import { useState, useEffect } from 'react';
import { Link } from 'react-router-dom';
import { api } from '../services/api';
import { useToken } from '../contexts/TokenContext';
import type { KeysResponse } from '../types';

export default function SecurityKeyList() {
  const { token: storedToken } = useToken();
  const [data, setData] = useState<KeysResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadKeys = async () => {
    if (!storedToken) return;

    setLoading(true);
    setError(null);
    try {
      const response = await api.listKeys(storedToken);
      setData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load keys');
      setData(null);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (storedToken) {
      loadKeys();
    }
  }, [storedToken]);

  return (
    <div>
      <div className="page-header">
        <h2>Security Keys</h2>
        <p>List all security keys (requires admin token)</p>
      </div>

      {!storedToken && (
        <div className="alert alert-error">
          No API token configured. <Link to="/settings">Configure your token in Settings</Link>
        </div>
      )}

      {storedToken && (
        <div className="card">
          <button className="btn btn-primary" onClick={loadKeys} disabled={loading}>
            {loading ? 'Loading...' : 'Refresh Keys'}
          </button>
        </div>
      )}

      {error && <div className="alert alert-error">{error}</div>}

      {data && (
        <div className="card">
          <div className="table-container">
            {data.length > 0 ? (
              <table>
                <thead>
                  <tr>
                    <th>ID</th>
                    <th>Created</th>
                  </tr>
                </thead>
                <tbody>
                  {data.map((key) => (
                    <tr key={key.id}>
                      <td>{key.id}</td>
                      <td>{new Date(key.created).toLocaleString()}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            ) : (
              <div className="empty-state">
                <h3>No keys found</h3>
                <p>Create a key to get started</p>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
