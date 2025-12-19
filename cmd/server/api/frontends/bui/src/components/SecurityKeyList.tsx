import { useState } from 'react';
import { api } from '../services/api';
import type { KeysResponse } from '../types';

export default function SecurityKeyList() {
  const [token, setToken] = useState('');
  const [data, setData] = useState<KeysResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token.trim()) return;

    setLoading(true);
    setError(null);
    try {
      const response = await api.listKeys(token.trim());
      setData(response);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load keys');
      setData(null);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div>
      <div className="page-header">
        <h2>Security Keys</h2>
        <p>List all security keys (requires admin token)</p>
      </div>

      <div className="card">
        <form onSubmit={handleSubmit}>
          <div className="form-group">
            <label htmlFor="adminToken">Admin Token</label>
            <input
              type="password"
              id="adminToken"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder="Enter admin token (KRONK_TOKEN)"
            />
          </div>
          <button className="btn btn-primary" type="submit" disabled={loading || !token.trim()}>
            {loading ? 'Loading...' : 'List Keys'}
          </button>
        </form>
      </div>

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
